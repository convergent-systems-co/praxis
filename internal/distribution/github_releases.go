package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

const (
	ManifestAssetName = "praxis-package.json"
	ArtifactAssetName = "praxis-package.tar.gz"
)

type GitHubReleases struct {
	Client  *http.Client
	APIBase string
	Token   string
}

func (g GitHubReleases) client() *http.Client {
	if g.Client != nil { return g.Client }
	return http.DefaultClient
}
func (g GitHubReleases) base() string {
	if g.APIBase != "" { return strings.TrimRight(g.APIBase,"/") }
	return "https://api.github.com"
}
func (g GitHubReleases) request(ctx context.Context, method, rawURL string) (*http.Response,error) {
	req,err:=http.NewRequestWithContext(ctx,method,rawURL,nil); if err!=nil{return nil,err}
	req.Header.Set("Accept","application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version","2022-11-28")
	if g.Token!="" { req.Header.Set("Authorization","Bearer "+g.Token) }
	return g.client().Do(req)
}
func decodeResponse(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	if resp.StatusCode<200 || resp.StatusCode>=300 { body,_:=io.ReadAll(io.LimitReader(resp.Body,4096)); return fmt.Errorf("github response %s: %s",resp.Status,strings.TrimSpace(string(body))) }
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (g GitHubReleases) Discover(ctx context.Context, query string) ([]Candidate,error) {
	q:="topic:praxis-plugin"
	if strings.TrimSpace(query)!="" { q += " "+strings.TrimSpace(query) }
	u:=g.base()+"/search/repositories?q="+url.QueryEscape(q)+"&per_page=50"
	resp,err:=g.request(ctx,http.MethodGet,u); if err!=nil{return nil,err}
	var payload struct{ Items []struct{ FullName string `json:"full_name"`; Description string `json:"description"`; HTMLURL string `json:"html_url"` } `json:"items"` }
	if err:=decodeResponse(resp,&payload); err!=nil{return nil,err}
	out:=make([]Candidate,0,len(payload.Items))
	for _,item:=range payload.Items { parts:=strings.SplitN(item.FullName,"/",2); if len(parts)!=2{continue}; out=append(out,Candidate{Ref:PackageRef{Source:"github-releases",Owner:parts[0],Repo:parts[1]},Description:item.Description,WebURL:item.HTMLURL}) }
	return out,nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets []struct{ Name string `json:"name"`; BrowserDownloadURL string `json:"browser_download_url"` } `json:"assets"`
}

func (g GitHubReleases) Info(ctx context.Context, ref PackageRef, version string) (Release,error) { return g.Resolve(ctx,ref,version) }

func (g GitHubReleases) Resolve(ctx context.Context, ref PackageRef, version string) (Release,error) {
	if err:=ref.Validate(); err!=nil{return Release{},err}
	var endpoint string
	if version=="" || version=="latest" { endpoint=fmt.Sprintf("%s/repos/%s/%s/releases/latest",g.base(),url.PathEscape(ref.Owner),url.PathEscape(ref.Repo)) } else { endpoint=fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s",g.base(),url.PathEscape(ref.Owner),url.PathEscape(ref.Repo),url.PathEscape(version)) }
	resp,err:=g.request(ctx,http.MethodGet,endpoint); if err!=nil{return Release{},err}
	var gh githubRelease; if err:=decodeResponse(resp,&gh); err!=nil{return Release{},err}
	result:=Release{Ref:ref,Tag:gh.TagName,WebURL:gh.HTMLURL}
	for _,asset:=range gh.Assets { switch asset.Name { case ManifestAssetName: result.ManifestURL=asset.BrowserDownloadURL; case ArtifactAssetName: result.ArtifactURL=asset.BrowserDownloadURL } }
	if result.ManifestURL=="" { return Release{},fmt.Errorf("release %q missing %s",gh.TagName,ManifestAssetName) }
	if result.ArtifactURL=="" { return Release{},fmt.Errorf("release %q missing %s",gh.TagName,ArtifactAssetName) }
	manifestResp,err:=g.request(ctx,http.MethodGet,result.ManifestURL); if err!=nil{return Release{},err}
	defer manifestResp.Body.Close(); if manifestResp.StatusCode<200||manifestResp.StatusCode>=300{return Release{},fmt.Errorf("manifest download response %s",manifestResp.Status)}
	if err:=json.NewDecoder(manifestResp.Body).Decode(&result.Manifest); err!=nil{return Release{},fmt.Errorf("decode package manifest: %w",err)}
	if err:=result.Manifest.Validate(); err!=nil{return Release{},fmt.Errorf("invalid package manifest: %w",err)}
	return result,nil
}

func (g GitHubReleases) FetchArtifact(ctx context.Context, release Release) ([]byte,error) {
	if release.ArtifactURL=="" { return nil,errors.New("release artifact URL is required") }
	resp,err:=g.request(ctx,http.MethodGet,release.ArtifactURL); if err!=nil{return nil,err}; defer resp.Body.Close()
	if resp.StatusCode<200||resp.StatusCode>=300{return nil,fmt.Errorf("artifact download response %s",resp.Status)}
	return io.ReadAll(resp.Body)
}

func (g GitHubReleases) CheckUpdate(ctx context.Context, ref PackageRef, installedVersion string) (Release,bool,error) {
	latest,err:=g.Resolve(ctx,ref,"latest"); if err!=nil{return Release{},false,err}
	return latest, latest.Manifest.Version!=installedVersion, nil
}

var _ Adapter = GitHubReleases{}
var _ = packagecatalog.Manifest{}

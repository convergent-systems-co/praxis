package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

type InstalledPackage struct {
	Manifest   packagecatalog.Manifest `json:"manifest"`
	State      string                  `json:"state"`
	SourceKind string                  `json:"source_kind"`
	SourceRef  string                  `json:"source_ref"`
}

func (s *Store) InstalledPackages(ctx context.Context) ([]InstalledPackage,error) {
	if s==nil||s.db==nil{return nil,errors.New("state store is required")}
	rows,err:=s.db.QueryContext(ctx,`SELECT manifest_json,state,source_kind,source_ref FROM installed_packages WHERE state<>'removed' ORDER BY package_id,package_version`); if err!=nil{return nil,err}; defer rows.Close()
	var out []InstalledPackage
	for rows.Next(){ var body []byte; var p InstalledPackage; if err:=rows.Scan(&body,&p.State,&p.SourceKind,&p.SourceRef);err!=nil{return nil,err}; if err:=json.Unmarshal(body,&p.Manifest);err!=nil{return nil,fmt.Errorf("decode installed package: %w",err)}; out=append(out,p) }
	return out,rows.Err()
}

func (s *Store) ActivePackage(ctx context.Context, packageID string) (InstalledPackage,error) {
	if s==nil||s.db==nil||packageID==""{return InstalledPackage{},errors.New("state store and package id are required")}
	var body []byte; var p InstalledPackage
	if err:=s.db.QueryRowContext(ctx,`SELECT manifest_json,state,source_kind,source_ref FROM installed_packages WHERE package_id=? AND state='active'`,packageID).Scan(&body,&p.State,&p.SourceKind,&p.SourceRef);err!=nil{return InstalledPackage{},err}
	if err:=json.Unmarshal(body,&p.Manifest);err!=nil{return InstalledPackage{},fmt.Errorf("decode installed package: %w",err)}
	return p,nil
}

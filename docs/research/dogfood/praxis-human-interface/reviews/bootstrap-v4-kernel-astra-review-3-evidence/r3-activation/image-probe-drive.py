import os,subprocess,hashlib,json,shutil,sys,time,tempfile
P=os.path.dirname(os.path.abspath(__file__))
def sha(p): return "sha256:"+hashlib.sha256(open(p,'rb').read()).hexdigest()
def ident():
    d=dict(l.split() for l in subprocess.check_output([P+"/probe-A","identity"],text=True).splitlines())
    return d["vcs.revision"], d["vcs.modified"]=="true"
rev,mod=ident()
def manifest(d,path,digest):
    f=tempfile.mktemp(dir=d,suffix=".json")
    json.dump({"active_binary_path":path,"active_binary_digest":digest,"source_commit":rev,"build_modified":mod},open(f,"w"))
    return f
def scenario(name, launch_rel, manifest_path_rel, manifest_digest_of, mutate, env=None, wait_marker=None):
    d=tempfile.mkdtemp(prefix="scn-")
    shutil.copy(P+"/probe-A",d+"/active"); os.chmod(d+"/active",0o755)
    shutil.copy(P+"/probe-B",d+"/B"); shutil.copy(P+"/probe-A",d+"/Acopy")
    pre=mutate.get("pre")
    if pre: pre(d)
    mf=manifest(d,d+"/"+manifest_path_rel,sha(P+"/probe-"+manifest_digest_of))
    e=dict(os.environ); e.update(env or {})
    p=subprocess.Popen([d+"/"+launch_rel,"run",mf],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True,env=e)
    if wait_marker:
        for _ in range(100):
            if os.path.exists(wait_marker): break
            time.sleep(0.05)
    else:
        first=p.stdout.readline().strip()
    try:
        mutate["do"](d)
        mut="ok"
    except Exception as ex:
        mut="MUTATION-FAILED: %r"%ex
    try:
        p.stdin.write("go\n"); p.stdin.flush()
    except Exception: pass
    out=p.stdout.read().strip(); p.wait()
    print("### %s [mutation %s]\n%s\n"%(name,mut,out))
    shutil.rmtree(d,ignore_errors=True)
def rename_B(d): os.rename(d+"/B",d+"/active")
def unlink_recreate(d):
    os.unlink(d+"/active"); shutil.copy(d+"/B",d+"/active"); os.chmod(d+"/active",0o755)
def inplace_trunc(d):
    with open(d+"/active","r+b") as f:
        f.truncate(0); f.write(open(d+"/B","rb").read())
def inplace_same_size_partial(d):
    b=open(d+"/B","rb").read()
    with open(d+"/active","r+b") as f:
        f.seek(0); f.write(b)   # same size overwrite
def identical_new_inode(d):
    shutil.copy(d+"/Acopy",d+"/tmpnew"); os.chmod(d+"/tmpnew",0o755); os.rename(d+"/tmpnew",d+"/active")
def noop(d): pass
scenario("control: no mutation, manifest for A","active","active","A",{"do":noop})
scenario("atomic rename B over path, manifest binds B","active","active","B",{"do":rename_B})
scenario("atomic rename B over path, manifest binds A","active","active","A",{"do":rename_B})
scenario("unlink+recreate B, manifest binds B","active","active","B",{"do":unlink_recreate})
scenario("unlink+recreate B, manifest binds A","active","active","A",{"do":unlink_recreate})
scenario("in-place truncate+write B (running exe), manifest binds B","active","active","B",{"do":inplace_trunc})
scenario("in-place same-size overwrite B, manifest binds B","active","active","B",{"do":inplace_same_size_partial})
scenario("in-place same-size overwrite B, manifest binds A","active","active","A",{"do":inplace_same_size_partial})
scenario("identical bytes, new inode (cp+rename), manifest binds A","active","active","A",{"do":identical_new_inode})
def mklinks(d):
    os.link(d+"/active",d+"/hard"); os.symlink(d+"/active",d+"/sym")
scenario("launched via hardlink, manifest path=active (same inode)","hard","active","A",{"pre":mklinks,"do":noop})
scenario("launched via symlink, manifest path=symlink","sym","sym","A",{"pre":mklinks,"do":noop})
scenario("launched via symlink, manifest path=real","sym","active","A",{"pre":mklinks,"do":noop})
def swap_symlink(d):
    os.unlink(d+"/sym"); os.symlink(d+"/B",d+"/sym")
scenario("launched via symlink, symlink retargeted to B, manifest path=sym binds B","sym","sym","B",{"pre":mklinks,"do":swap_symlink})
# exec->init window
sf=tempfile.mktemp(prefix="sleepflag-")
scenario("EXEC-TO-INIT WINDOW: A execs, path replaced by B before bootstrapv4 init, manifest binds B","active","active","B",{"do":rename_B},env={"PROBE_SLEEP_FILE":sf},wait_marker=sf)
os.path.exists(sf) and os.unlink(sf)

#!/usr/bin/env python3
"""Consolidated guard/mutation harness. Mutates ONLY scratch copies of the repo.

Edit kinds
  ("neg", file, anchor, nth, off)   disable the condition of the `if` on the anchor line (+off lines):
                                    `if init; cond {` -> `if init; false && (cond) {`
  ("sub", file, old, new, nth)      replace the nth occurrence of old with new
A mutation is a list of edits (joint mutation). KILLED = >=1 relevant test failed; SURVIVED = all green;
BUILD-FAILED / NO-TESTS / ANCHOR-* are reported and never counted.
"""
import json, os, re, subprocess, sys, shutil, hashlib, threading, queue, time
REAL = "/Users/polliard/workspace/convergent-systems-co/praxis"
HERE = os.path.dirname(os.path.abspath(__file__))
COPIES = int(os.environ.get("COPIES", "3"))

def neg(line):
    m = re.search(r'(else )?if ', line)
    if not m: raise ValueError("no if on line: "+line)
    start = m.end()
    rest = line[start:]
    semi = rest.find('; ')
    if semi >= 0:
        init, cond = rest[:semi+2], rest[semi+2:]
    else:
        init, cond = "", rest
    cond = cond.rstrip()
    assert cond.endswith('{'), line
    cond = cond[:-1].rstrip()
    return line[:start] + init + "false && (" + cond + ") {"

def apply_edit(text, edit):
    kind = edit[0]
    if kind == "neg":
        _, f, anchor, nth, off = edit
        lines = text.split("\n")
        idxs = [i for i, l in enumerate(lines) if anchor in l]
        if len(idxs) < nth: raise LookupError("ANCHOR-NOT-FOUND %r (%d found)" % (anchor, len(idxs)))
        if nth == 1 and len(idxs) > 1 and not edit_allow_multi: raise LookupError("ANCHOR-AMBIGUOUS %r (%d found)" % (anchor, len(idxs)))
        i = idxs[nth-1] + off
        lines[i] = neg(lines[i])
        return "\n".join(lines)
    if kind == "sub":
        _, f, old, new, nth = edit
        parts = text.split(old)
        if len(parts) - 1 < nth: raise LookupError("ANCHOR-NOT-FOUND %r (%d found)" % (old[:60], len(parts)-1))
        return old.join(parts[:nth]) + new + old.join(parts[nth:])
    raise ValueError(kind)
edit_allow_multi = True

def sha(p): return hashlib.sha256(open(p,'rb').read()).hexdigest()

def sync(copy):
    if not os.path.isdir(copy):
        os.makedirs(copy)
    subprocess.run(["rsync","-a","--delete","--exclude","graphify-out","--exclude",".ai","--exclude","/praxis","--exclude","docs/research/dogfood/praxis-human-interface/reviews","REAL/".replace("REAL",REAL),copy+"/"],check=True)

def run_one(copy, mut):
    changed = {}
    try:
        for e in mut["edits"]:
            f = e[1]
            p = os.path.join(copy, f)
            if f not in changed: changed[f] = open(p).read()
            cur = open(p).read()
            open(p,"w").write(apply_edit(cur, e))
    except LookupError as ex:
        for f, t in changed.items(): open(os.path.join(copy,f),"w").write(t)
        return "ANCHOR-ERROR", str(ex), "", []
    try:
        cmd = ["go","test","-v","-count=1","-timeout=600s","-run",mut["run"]] + mut["pkgs"]
        p = subprocess.run(cmd, cwd=copy, capture_output=True, text=True, timeout=1500)
        out = p.stdout + p.stderr
    finally:
        for f, t in changed.items(): open(os.path.join(copy,f),"w").write(t)
    failed = sorted(set(l.split()[2] for l in out.splitlines() if l.startswith("--- FAIL")))
    skipped = sorted(set(l.split()[2] for l in out.splitlines() if l.startswith("--- SKIP")))
    if "[build failed]" in out or "setup failed" in out: v = "BUILD-FAILED"
    elif p.returncode == 0:
        v = "NO-TESTS" if ("[no tests to run]" in out and "ok" not in out.replace("[no tests to run]","")) else "SURVIVED"
    else: v = "KILLED" if failed or "panic:" in out or "FAIL" in out else "ERROR"
    return v, ",".join(failed[:6]), (out[-600:] if v in ("BUILD-FAILED","ERROR") else ""), skipped

def main():
    muts = json.load(open(sys.argv[1]))
    sel = sys.argv[2:]
    if sel: muts = [m for m in muts if any(m["id"].startswith(s) for s in sel)]
    out_path = os.path.join(HERE, os.environ.get("OUT","results.json"))
    done = {}
    if os.path.exists(out_path) and not os.environ.get("FRESH"):
        done = {r["id"]: r for r in json.load(open(out_path))}
    todo = [m for m in muts if m["id"] not in done or done[m["id"]]["verdict"] in ("BUILD-FAILED","ANCHOR-ERROR","NO-TESTS","ERROR")]
    copies = [os.path.join(HERE, "copyb%d" % i) for i in range(COPIES)]
    for c in copies: sync(c)
    q = queue.Queue()
    for m in todo: q.put(m)
    lock = threading.Lock()
    def worker(copy):
        while True:
            try: m = q.get_nowait()
            except queue.Empty: return
            t0 = time.time()
            v, killers, tail, skipped = run_one(copy, m)
            rec = {"id": m["id"], "guard": m["guard"], "invariants": m["inv"], "group": m.get("group",""), "edits": [list(e) for e in m["edits"]], "run": m["run"], "pkgs": m["pkgs"], "verdict": v, "killers": killers, "skipped": skipped, "tail": tail, "secs": round(time.time()-t0,1)}
            with lock:
                done[m["id"]] = rec
                json.dump(list(done.values()), open(out_path,"w"), indent=1)
                print("%-13s %-8s %s  %s" % (v, m["id"], m["guard"][:60], killers[:90]), flush=True)
    ts = [threading.Thread(target=worker, args=(c,)) for c in copies]
    [t.start() for t in ts]; [t.join() for t in ts]
    # verify scratch copies restored identical to real for every touched file
    bad = []
    for m in muts:
        for e in m["edits"]:
            for c in copies:
                a, b = os.path.join(c, e[1]), os.path.join(REAL, e[1])
                if os.path.exists(a) and sha(a) != sha(b) and (e[1], c) not in bad: bad.append((e[1], c))
    print("RESTORE-CHECK", "OK" if not bad else bad)

if __name__ == "__main__": main()

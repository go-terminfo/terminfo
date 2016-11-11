#!/usr/bin/python3
# NOTE this is probably wrong-headed

import contextlib
import os
import re

# note the sneaky s and the end of type
match = re.compile(r"^#define\s+(?P<name>\S+)\s+CUR\s+(?P<type>\w+)s\[(?P<index>\d+)\]\s*$").match
py2cc_sub = re.compile(r"(?:^|_)(.)").sub

def camelcase(s):
    return py2cc_sub(lambda m: m.group(1).upper(), s)

attrs = dict(Boolean=[], Number=[], String=[])
for line in open("/usr/include/term.h"):
    m = match(line)
    if m is None:
        continue
    d = m.groupdict()
    tl = attrs[d["type"]]
    idx = int(d["index"])
    while len(tl) <= idx:
        tl.append("")
    tl[idx] = camelcase(d["name"])

fn = os.path.join(os.path.dirname(__file__), "termh.go")
with contextlib.redirect_stdout(open(fn, "w")):
    print("// GENERATED FILE -- DO NOT EDIT")
    print("// fix gen-termh.py instead")
    print()
    print("package terminfo")
    for typ, bs in attrs.items():
        print("type {}Index int".format(typ))
        print("const (")
        print("\t{} {}Index = iota".format(bs[0], typ))
        for b in bs[1:]:
            print("\t{}".format(b))
        print("\tMax{}Index = {}".format(typ, b))
        print(")")

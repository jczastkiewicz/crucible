import os,re,sys,collections
go=[]
for d,_,fs in os.walk('crucible/internal'):
    for f in fs:
        if f.endswith('.go') and not f.endswith('_test.go'):
            go.append(open(os.path.join(d,f),encoding='utf-8').read().lower())
allgo='\n'.join(go)
def named(tok):
    t=tok.lower()
    return f'"{t}"' in allgo or f'{t}$' in allgo or f'"{t}:' in allgo or f'"{t} ' in allgo or f'`{t}' in allgo
keyre=re.compile(r'(?:^|\|)\s*(\w+)\$')
valre=re.compile(r'(\w+)\$\s*([^|]*)')
cnt=re.compile(r'Count\$(\w+)')
basics={'Forest','Island','Mountain','Plains','Swamp'}
hits=collections.defaultdict(set)
cards=0
for line in open(sys.argv[1]):
    name,p=line.rstrip('\n').split('\t')
    if name in basics: continue
    cards+=1
    for l in open(p,encoding='utf-8'):
        l=l.rstrip('\n')
        if l.startswith(('Oracle:','Name:','ManaCost:','Types:','PT:','Text:','Loyalty:','Defense:','Colors:','ALTERNATE','AlternateMode','SVar:AIPreference','SVar:PlayMain1','DeckHas','DeckHints','DeckNeeds','SVar:Pick','SVar:Hidden','AI:','Variant','SpecializeName')): continue
        if l.startswith('SVar:') and re.match(r'SVar:(AI|Deck|Pick|Play|Hidden|Must|Never|Sacr|End|Targeting|Has|NonStacking|Chosen|Is|Buffed|Remove)', l): continue
        toks=set()
        if l.startswith('K:'):
            toks.add('K:'+l[2:].split(':')[0].split(' ')[0] if ':' in l[2:] else 'K:'+l[2:])
        for k in keyre.findall(l): toks.add('$'+k)
        for k,v in valre.findall(l):
            if k in ('Mode','Event','AB','SP','DB','ApiType','Affected','ValidCard','ValidTgts','Defined','Cost','CostDesc'):
                pass
            if k in ('Mode','Event','AB','SP','DB'): toks.add(k+'='+v.strip().split(' ')[0])
        for c in cnt.findall(l): toks.add('Count$'+c)
        for t in toks:
            core=t.split('=',1)[-1].split('$',1)[-1].split(':',1)[-1]
            if core and not named(core): hits[t].add(name)
print(f"# {cards} cards swept", file=sys.stderr)
for t,ns in sorted(hits.items(), key=lambda x:(-len(x[1]),x[0])):
    print(f"{t}\t{len(ns)}\t{'; '.join(sorted(ns))}")

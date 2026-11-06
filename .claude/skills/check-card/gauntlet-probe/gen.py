# gen.py SPEC OUTDIR: one probe scenario per "== Card" block.
# Block lines: "cost W U 1" (pips to tap: W U B R G C; generic as numbers -> Wastes),
# "set key=value" (replace/extend a setup key, '+' appends: set +aibattlefield=X|Id:55),
# anything else is an action line after castspell; "pre ..." before castspell;
# "nocast" skips castspell (land/board probes); "play" plays the card as a land.
import sys,os,re,shutil
LAND={'W':'Plains','U':'Island','B':'Swamp','R':'Mountain','G':'Forest','C':'Wastes'}
def base():
    return {'humanlife':'20','ailife':'20',
     'humanlibrary':';'.join(f'Island|Id:{i}' for i in range(80,86)),
     'ailibrary':';'.join(f'Island|Id:{i}' for i in range(90,96)),
     'humangraveyard':'Lightning Bolt|Id:70;Grizzly Bears|Id:71',
     'aigraveyard':'Shock|Id:72;Hill Giant|Id:73',
     'aihand':'Island|Id:60;Lightning Bolt|Id:61;Grizzly Bears|Id:62',
     'aibattlefield':'Grizzly Bears|Id:50;Ornithopter|Id:51;Swamp|Id:52;Pacifism|Id:53|Attaching:50'}
def slug(n): return re.sub(r'[^a-z0-9]+','-',n.lower()).strip('-')
spec,out=sys.argv[1],sys.argv[2]
shutil.rmtree(out,ignore_errors=True); os.makedirs(out)
blocks=re.split(r'^== ',open(spec).read(),flags=re.M)[1:]
for b in blocks:
    lines=b.split('\n'); full=lines[0].strip(); name=full.split(' #')[0]; st=base(); pre=[]; post=[]; pips=[]; cast=True; play=False
    st['humanhand']=f'{name}|Id:1'
    for l in lines[1:]:
        l=l.strip()
        if not l or l.startswith('#'): continue
        if l.startswith('cost '):
            for t in l[5:].split():
                pips += ['X']*int(t) if t.isdigit() else [t]
        elif l.startswith('set '):
            k,v=l[4:].split('=',1)
            if k.startswith('+'): k=k[1:]; st[k]=(st.get(k,'')+';'+v).strip(';')
            else: st[k]=v
        elif l=='nocast': cast=False
        elif l=='play': play=True; cast=False
        elif l.startswith('pre '): pre.append(l[4:])
        elif l.startswith('queue '): pre.append(l)
        else: post.append(l)
    colored=[p for p in pips if p not in ('C','X')]
    gc=colored[0] if colored else 'U'
    gen=[]
    for i,p in enumerate(pips):
        if p=='X': pips[i]=gc; gen.append(gc)
    pre=[f'queue paygeneric {g}' for g in gen]+pre
    lands=[f'{LAND[p]}|Id:{100+i}' for i,p in enumerate(pips)]
    if lands: st['humanbattlefield']=(';'.join(lands)+';'+st.get('humanbattlefield','')).strip(';')
    acts=['startturn human','advance 3']+pre+[f'tapformana human {100+i} {p}' for i,p in enumerate(pips)]
    if cast: acts.append('castspell human 1')
    if play: acts.append('playland human 1')
    acts+=post if post else (['resolvestack'] if cast else [])
    d=os.path.join(out,'probe-'+slug(full)); os.makedirs(d)
    open(os.path.join(d,'card.txt'),'w').write(full)
    open(os.path.join(d,'setup.state'),'w').write(''.join(f'{k}={v}\n' for k,v in st.items() if v))
    open(os.path.join(d,'actions.log'),'w').write('\n'.join(acts)+'\n')

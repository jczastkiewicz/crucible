# promote.py PROBEDIR L2DIR SCENARIOS [skip...]: every "same" probe becomes gauntlet-<slug>.
import sys,os,re,shutil
probe,l2,dst=sys.argv[1:4]; skip=set(sys.argv[4:])
rep=open(os.path.join(l2,'report.txt')).read()
bad=set(re.findall(r'^(probe-[\w-]+)',rep,re.M))
names=[f[:-6] for f in os.listdir(os.path.join(l2,'go')) if f.endswith('.state') and os.path.exists(os.path.join(l2,'java',f)) and f[:-6] not in bad]
n=0
for p in names:
    p=p.strip()
    if not p or p in skip: continue
    d=os.path.join(dst,'gauntlet-'+p[len('probe-'):])
    if os.path.exists(d): continue
    os.makedirs(d)
    shutil.copy(os.path.join(probe,p,'setup.state'),d)
    shutil.copy(os.path.join(l2,'go',p+'.state'),os.path.join(d,'expect.state'))
    acts=open(os.path.join(probe,p,'actions.log')).read()
    card=open(os.path.join(probe,p,'card.txt')).read().strip() if os.path.exists(os.path.join(probe,p,'card.txt')) else p
    open(os.path.join(d,'actions.log'),'w').write(f"# R20c gauntlet probe ({card}): the card played the way the Modern\n# gauntlet plays it, on a fixed board; L2: same as Java.\n"+acts)
    n+=1
print(n,'promoted')

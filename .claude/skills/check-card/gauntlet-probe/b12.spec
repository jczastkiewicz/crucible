== Atraxa, Grand Unifier
cost 3 G W U B
set humanlibrary=Island|Id:80;Grizzly Bears|Id:81;Lightning Bolt|Id:82
queue optionname Creature
queue cardchoice 81
queue optionname Instant
queue cardchoice 82
queue optionname Land
queue cardchoice 80
resolvestack
resolvestack
== The Legend of Roku
cost 2 R R
resolvestack
resolvestack
== The Legend of Roku #chapter2
nocast
set humanhand=
set humanbattlefield=The Legend of Roku|Id:1|Counters:LORE=1
queue manacolor R
resolvestack
== The Legend of Roku #chapter3
nocast
set humanhand=
set humanbattlefield=The Legend of Roku|Id:1|Counters:LORE=2
resolvestack
== Wish
cost 2 R
set humansideboard=Lightning Bolt|Id:40
set +humanbattlefield=Mountain|Id:110
queue targets ai
resolvestack
tapformana human 110 R
castspell human 40
resolvestack
== Emrakul, the Promised End
cost 11
queue targets ai
resolvestack
resolvestack
== Emrakul, the Promised End #stolen
cost 11
queue targets ai
resolvestack
resolvestack
advance 24
== Lightning Bolt #kraven-greatest
nocast
set humanbattlefield=Kraven the Hunter|Id:2;Mountain|Id:3
set aibattlefield=Hill Giant|Id:50;Grizzly Bears|Id:51
queue targets 50
tapformana human 3 R
castspell human 1
resolvestack
resolvestack
== Lightning Bolt #kraven-smaller
nocast
set humanbattlefield=Kraven the Hunter|Id:2;Mountain|Id:3
set aibattlefield=Hill Giant|Id:50;Grizzly Bears|Id:51
queue targets 51
tapformana human 3 R
castspell human 1
resolvestack
resolvestack
== Lightning Bolt #imodane-creature
nocast
set humanbattlefield=Imodane, the Pyrohammer|Id:2;Mountain|Id:3
queue targets 50
tapformana human 3 R
castspell human 1
resolvestack
resolvestack
== Lightning Bolt #imodane-player
nocast
set humanbattlefield=Imodane, the Pyrohammer|Id:2;Mountain|Id:3
queue targets ai
tapformana human 3 R
castspell human 1
resolvestack
resolvestack
== Electryte #defending-player
nocast
set humanbattlefield=Electryte|Id:2;Grizzly Bears|Id:3
set aibattlefield=Hill Giant|Id:50
advance 2
queue attackers 2,3
declareattackers
advance
queue blocks 50=3
declareblockers
advance
advance
combatdamage
resolvestack
advance
== Latulla's Orders #defending-player
nocast
set humanbattlefield=Grizzly Bears|Id:2;Latulla's Orders|Id:3|Attaching:2
set aibattlefield=Ornithopter|Id:51
advance 2
queue attackers 2
declareattackers
advance
queue blocks none
declareblockers
advance
advance
queue optionaltrigger true
queue targets 51
combatdamage
resolvestack
advance
== Miss Highwater #no-contract
nocast
set humanbattlefield=Miss Highwater|Id:2
set aibattlefield=Swamp|Id:52
set ailibrary=Island|Id:90;Island|Id:91;Island|Id:92;Island|Id:93;Island|Id:94;Island|Id:95;Island|Id:96;Island|Id:97;Island|Id:98;Island|Id:99;Island|Id:100;Island|Id:101
advance 2
queue attackers 2
declareattackers
advance
queue blocks none
declareblockers
advance
advance
queue confirmpaycost true
queue cardorder 60,61,62
combatdamage
resolvestack
advance
== Hellish Rebuke #caster-damaged
cost B 2
set humanhand=Hellish Rebuke|Id:1;Swamp|Id:6
set +aibattlefield=Ankh of Mishra|Id:54
resolvestack
playland human 6
resolvestack
resolvestack
== The Beast, Deathless Prince #owner-damaged
cost B R 2
set aibattlefield=Grizzly Bears|Id:50
queue targets 50
resolvestack
resolvestack
advance 2
queue attackers 50
declareattackers
advance
queue blocks none
declareblockers
advance
advance
combatdamage
resolvestack
advance
== Vivid Marsh #enters-tapped-charged
play
== Steel Dromedary #enters-tapped-counters
cost 3
== Sleep-Cursed Faerie #stun-untap-step
nocast
set humanbattlefield=Sleep-Cursed Faerie|Id:2|Tapped|Counters:STUN=3
advance 24

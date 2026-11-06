== Dryad Arbor
play
== Echoing Deeps
play
set +humangraveyard=Bojuka Bog|Id:74
queue confirmeffect true
queue cardchoice 74
queue targets ai
queue cardorder 72,73
resolvestack
== Emperor of Bones
nocast
set humanhand=
set humanbattlefield=Emperor of Bones|Id:1;Swamp|Id:100;Island|Id:101
queue targets 73
advance 1
resolvestack
tapformana human 100 B
tapformana human 101 U
queue paygeneric U
queue action human activate 1 0
queue cardchoice 73
passpriority
resolvestack
== Emry, Lurker of the Loch
cost 2 U
queue cardorder 80,81,82,83
resolvestack
resolvestack
== Endurance
cost 1 G G
queue targets ai
resolvestack
resolvestack
== Engineered Explosives
cost R G
queue payx 2
queue paygeneric R
queue paygeneric G
== Engineered Explosives #act
nocast
set humanhand=
set humanbattlefield=Engineered Explosives|Id:1|Counters:CHARGE=2
cost 2
queue action human activate 1 0
queue cardorder 50,53
passpriority
== Ephemerate
cost W
set +humanbattlefield=Grizzly Bears|Id:3
queue targets 3
resolvestack
== Erode
cost W
queue targets 50
queue confirmeffect true
queue cardchoice 90
resolvestack
== Explore
cost 1 G
== Expressive Iteration
cost U R
queue cardchoice 80
queue cardchoice 81
resolvestack
== Fatal Push
cost B
queue targets 50
resolvestack
== Fiery Islet
nocast
set humanhand=
set humanbattlefield=Fiery Islet|Id:1
cost 1
queue action human activate 1 1
passpriority
== Flickerwisp
cost 1 W W
queue targets 50
resolvestack
resolvestack
== Force of Negation
nocast
set humanhand=Force of Negation|Id:1;Lightning Bolt|Id:2
cost U U U R
queue paygeneric U
queue targets ai
castspell human 2
queue targets 2
castspell human 1
resolvestack
== Force of Vigor
cost 2 G G
queue targets 53,51
queue cardorder 53,51
resolvestack
== Galvanic Blast
cost R
queue targets ai
resolvestack
== Galvanic Blast #metalcraft
cost R
set +humanbattlefield=Ornithopter|Id:3;Ornithopter|Id:4;Ornithopter|Id:5
queue targets ai
resolvestack
== Galvanic Relay
cost 2 R
== Glaring Fleshraker
nocast
set humanhand=Ornithopter|Id:2
set humanbattlefield=Glaring Fleshraker|Id:1
castspell human 2
resolvestack
== Glistening Deluge
cost 1 B B
== Goblin Bombardment
nocast
set humanhand=
set humanbattlefield=Goblin Bombardment|Id:1;Grizzly Bears|Id:3
queue action human activate 1 0
queue sacrificechoice 3
queue targets ai
passpriority
== Goryo's Vengeance
cost 1 B
set humangraveyard=Griselbrand|Id:74
queue targets 74
resolvestack
== Grafdigger's Cage
nocast
set humanhand=Goryo's Vengeance|Id:2
set humangraveyard=Griselbrand|Id:74
set +humanbattlefield=Grafdigger's Cage|Id:1
cost 1 B
queue targets 74
castspell human 2
resolvestack
== Grapeshot
nocast
set humanhand=Grapeshot|Id:1;Lightning Bolt|Id:2
cost R R R
queue targets ai
castspell human 2
resolvestack
queue paygeneric R
queue targets ai
queue confirmeffect true
queue targets 50
castspell human 1
resolvestack
== Green Sun's Zenith
cost G G G
queue payx 2
queue paygeneric G
queue paygeneric G
set humanlibrary=Island|Id:80;Grizzly Bears|Id:81;Island|Id:82
queue cardchoice 81
resolvestack
== Griselbrand
nocast
set humanhand=
set humanbattlefield=Griselbrand|Id:1
set humanlibrary=Island|Id:80;Island|Id:81;Island|Id:82;Island|Id:83;Island|Id:84;Island|Id:85;Island|Id:86;Island|Id:87
queue action human activate 1 0
passpriority
== Gruul Turf
play
set humanbattlefield=Forest|Id:3
queue cardchoice 3
resolvestack
== Guide of Souls
nocast
set humanhand=Grizzly Bears|Id:2
set humanbattlefield=Guide of Souls|Id:1
cost G G
queue paygeneric G
castspell human 2
resolvestack
resolvestack

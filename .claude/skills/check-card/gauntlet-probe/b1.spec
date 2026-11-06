== Aftermath Analyst
cost 1 G
queue cardorder 80,81,82
resolvestack
resolvestack
== Aftermath Analyst #act
nocast
set humanhand=
set humanbattlefield=Aftermath Analyst|Id:1
set humangraveyard=Forest|Id:70;Plains|Id:71;Grizzly Bears|Id:72
cost 3 G
queue action human activate 1 0
queue cardorder 70,71
passpriority
== Ajani, Nacatl Pariah
cost 1 W
resolvestack
resolvestack
== Ajani, Nacatl Pariah #cat-dies
nocast
set humanhand=Lightning Bolt|Id:2
set humanbattlefield=Ajani, Nacatl Pariah|Id:1;Wild Nacatl|Id:3
cost R
queue targets 3
queue optionaltrigger true
castspell human 2
resolvestack
resolvestack
== Amulet of Vigor
nocast
set humanhand=Azorius Guildgate|Id:2
set humanbattlefield=Amulet of Vigor|Id:1
playland human 2
resolvestack
== Ancient Stirrings
cost G
set humanlibrary=Island|Id:80;Ornithopter|Id:81;Grizzly Bears|Id:82;Island|Id:83;Island|Id:84;Island|Id:85
queue cardchoice 81
queue cardorder 80,82,83,84
resolvestack
== Arboreal Grazer
cost G
set humanhand=Arboreal Grazer|Id:1;Forest|Id:2
queue optionaltrigger true
queue cardchoice 2
resolvestack
resolvestack
== Arena of Glory
play
== Arena of Glory #mountain
play
set humanbattlefield=Mountain|Id:3
== Arid Mesa
nocast
set humanhand=
set humanbattlefield=Arid Mesa|Id:1
set humanlibrary=Island|Id:80;Mountain|Id:81;Island|Id:82;Island|Id:83
queue action human activate 1 0
queue cardchoice 81
passpriority
== Artist's Talent
nocast
set humanhand=Lightning Bolt|Id:2;Island|Id:3
set humanbattlefield=Artist's Talent|Id:1
cost R
queue targets ai
castspell human 2
queue optionaltrigger true
queue confirmpaycost true
queue discardchoice 3
resolvestack
resolvestack
== Ashiok, Dream Render
nocast
set humanhand=
set humanbattlefield=Ashiok, Dream Render|Id:1|Counters:LOYALTY=5
queue cardorder 90,91,92,93
queue cardorder 72,73,90,91,92,93
queue action human activate 1 0
queue targets ai
passpriority
== Atraxa, Grand Unifier
cost 3 G W U B
set humanlibrary=Ornithopter|Id:80;Grizzly Bears|Id:81;Lightning Bolt|Id:82;Island|Id:83;Pacifism|Id:84;Divination|Id:85;Island|Id:86;Shock|Id:87;Island|Id:88;Island|Id:89;Island|Id:79
resolvestack
resolvestack
== Ba Sing Se
play
== Ba Sing Se #basic
play
set humanbattlefield=Forest|Id:3
== Basking Broodscale
nocast
set humanhand=
set humanbattlefield=Basking Broodscale|Id:1
cost 1 G
queue action human activate 1 0
queue optionaltrigger true
passpriority
resolvestack
== Blade of the Bloodchief
nocast
set humanhand=Lightning Bolt|Id:2
set humanbattlefield=Grizzly Bears|Id:3;Blade of the Bloodchief|Id:1|Attaching:3
cost R
queue targets 50
castspell human 2
resolvestack
resolvestack
== Blood Crypt #pay
play
queue confirmpaycost true
== Blood Crypt #nopay
play
queue confirmpaycost false
== Blood Moon
cost 2 R
set +humanbattlefield=Blood Crypt|Id:3
== Boggart Trawler
cost 2 B
queue cardorder 72,73
queue targets ai
resolvestack
resolvestack
== Bojuka Bog
play
queue targets ai
queue cardorder 72,73
resolvestack
== Boomerang Basics #own
cost U
set +humanbattlefield=Grizzly Bears|Id:3
queue targets 3
resolvestack
== Boomerang Basics #opp
cost U
queue targets 50
resolvestack
== Boseiju, Who Endures
nocast
cost 1 G
queue action human activate 1 1
queue targets 53
queue confirmeffect true
queue cardchoice 90
passpriority
== Break the Ice
cost B B
set +aibattlefield=Wastes|Id:54
queue targets 54
resolvestack
== Consign to Memory
nocast
queue numberchoice 0
set humanhand=Consign to Memory|Id:1;Ornithopter|Id:2
cost U
castspell human 2
queue targets 2
castspell human 1
resolvestack
== Consult the Star Charts
cost 1 U
queue confirmpaycost false
queue cardchoice 80
resolvestack
== Cori-Steel Cutter
nocast
set humanhand=Lightning Bolt|Id:2;Lightning Bolt|Id:3
set humanbattlefield=Cori-Steel Cutter|Id:1
cost R R
queue targets ai
queue targets ai
castspell human 2
castspell human 3
queue confirmeffect true
resolvestack
== Cultivator Colossus
cost 4 G G G
set humanhand=Cultivator Colossus|Id:1;Forest|Id:2
set humanlibrary=Grizzly Bears|Id:80;Grizzly Bears|Id:81
queue optionaltrigger true
queue cardchoice 2
resolvestack
resolvestack
== Dalkovan Encampment
play
== Damping Sphere
cost 2
== Dark Matter Manipulator
cost B
queue cardorder 80,81,82
resolvestack
resolvestack
== Day's Undoing
cost 2 U
== Defense Grid
cost 2
== Desperate Ritual
cost 1 R
== Detective's Phoenix
cost 2 R
== Devourer of Destiny
cost 7
set +humanbattlefield=Eldrazi Temple|Id:3
queue targets 50
resolvestack
resolvestack
== Dismember
cost 1 B B
queue payphyrexian false
queue payphyrexian false
queue targets 50
resolvestack

== Preordain
cost U
queue scry 81/80
resolvestack
== Thraben Charm
cost 1 W
queue modechoice 1
queue targets 53
resolvestack
== Pick Your Poison
cost G
queue modechoice 1
queue sacrificechoice 53
resolvestack
== Brotherhood's End
cost 1 R R
queue modechoice 1
resolvestack
== Brotherhood's End #damage
cost 1 R R
queue modechoice 0
queue cardorder 50,51
resolvestack
== Meltdown
cost R
queue payx 0
resolvestack
== Prismatic Ending
cost 1 W
queue payx 1
queue targets 53
resolvestack
== Mutagenic Growth
queue payphyrexian false
queue targets 50
resolvestack
== Mutagenic Growth #mana
cost G
queue payphyrexian true
queue targets 50
resolvestack
== Thoughtseize
cost B
queue targets ai
queue cardchoice 61
resolvestack
== Faithful Mending
cost W U
queue discardchoice 80,81
queue cardorder 80,81
resolvestack
== Galvanic Discharge
cost R
queue targets 50
queue numberchoice 2
queue confirmpaycost true
resolvestack
== Wrath of the Skies
cost 1 W W
queue payx 1
queue numberchoice 1
queue confirmpaycost true
resolvestack
== Flooded Strand
nocast
set humanhand=
set humanbattlefield=Flooded Strand|Id:1
queue action human activate 1 0
queue cardchoice 80
passpriority
== Polluted Delta
nocast
set humanhand=
set humanbattlefield=Polluted Delta|Id:1
queue action human activate 1 0
queue cardchoice 80
passpriority
== Scalding Tarn
nocast
set humanhand=
set humanbattlefield=Scalding Tarn|Id:1
queue action human activate 1 0
queue cardchoice 80
passpriority
== Godless Shrine
play
queue confirmpaycost true
== Steam Vents
play
queue confirmpaycost false
== Commercial District
play
queue surveil /80
resolvestack
== Meticulous Archive
play
queue surveil 80
resolvestack
== Dragon's Rage Channeler
nocast
set humanhand=Lightning Bolt|Id:2
set humanbattlefield=Dragon's Rage Channeler|Id:1
cost R
queue targets ai
queue surveil /80
castspell human 2
resolvestack
resolvestack
== Monastery Swiftspear
nocast
set humanhand=Lightning Bolt|Id:2
set humanbattlefield=Monastery Swiftspear|Id:1
cost R
queue targets ai
castspell human 2
resolvestack
resolvestack
== Talisman of Resilience
cost 2
== Springleaf Drum
cost 1
== Ocelot Pride
cost W
== Prismatic Ending #converge
cost W U
queue payx 1
queue paygeneric U
queue targets 53
resolvestack
== Prismatic Ending #zero
cost W
queue payx 0
queue targets 51
resolvestack
== Surgical Extraction #choose
queue payphyrexian false
queue targets 72
queue cardchoice 72
resolvestack
== Wrath of the Skies #two
cost 2 W W
queue payx 2
queue numberchoice 2
queue confirmpaycost true
queue cardorder 50,51,53
resolvestack

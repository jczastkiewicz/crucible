== Manamorphose
cost 1 R
queue hybridmanacolor R
queue manacolor G
queue manacolor U
resolvestack
== Street Wraith
nocast
set humanhand=Street Wraith|Id:1
queue action human activate 1 0
passpriority
== Walking Ballista
cost 2
queue payx 1
resolvestack
== Primeval Titan
cost 4 G G
set humanlibrary=Island|Id:80;Forest|Id:81;Forest|Id:82;Island|Id:83
queue optionaltrigger true
queue cardchoice 81,82
resolvestack
resolvestack
== Valakut Awakening
cost 2 R
set humanhand=Valakut Awakening|Id:1;Island|Id:2;Mountain|Id:3
queue cardchoice 2,3
queue cardorder 2,3
resolvestack

---- MODULE CommandIdempotency ----
EXTENDS Naturals, FiniteSets

CONSTANT Keys

VARIABLES generation, applied, effects

vars == <<generation, applied, effects>>

Init ==
    /\ generation = 0
    /\ applied = {}
    /\ effects = {}

Apply(k) ==
    /\ k \in Keys
    /\ k \notin applied
    /\ generation' = generation + 1
    /\ applied' = applied \cup {k}
    /\ effects' = effects \cup {k}

Replay(k) ==
    /\ k \in applied
    /\ UNCHANGED vars

Next ==
    \/ \E k \in Keys : Apply(k)
    \/ \E k \in Keys : Replay(k)

TypeOK ==
    /\ generation \in Nat
    /\ applied \subseteq Keys
    /\ effects \subseteq Keys

GenerationMatchesApplied ==
    generation = Cardinality(applied)

EffectsExactlyOnce ==
    effects = applied

Spec == Init /\ [][Next]_vars

====

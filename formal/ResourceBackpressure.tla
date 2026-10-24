---- MODULE ResourceBackpressure ----
EXTENDS Naturals, FiniteSets

CONSTANT Keys, Capacity

VARIABLES used, accepted, retryable

vars == <<used, accepted, retryable>>

Init ==
    /\ used = 0
    /\ accepted = {}
    /\ retryable = {}

Admit(k) ==
    /\ k \in Keys
    /\ k \notin accepted
    /\ used < Capacity
    /\ used' = used + 1
    /\ accepted' = accepted \cup {k}
    /\ retryable' = retryable \ {k}

Backpressure(k) ==
    /\ k \in Keys
    /\ k \notin accepted
    /\ used = Capacity
    /\ used' = used
    /\ accepted' = accepted
    /\ retryable' = retryable \cup {k}

Release ==
    /\ used > 0
    /\ used' = used - 1
    /\ UNCHANGED <<accepted, retryable>>

Replay(k) ==
    /\ k \in accepted
    /\ UNCHANGED vars

Next ==
    \/ \E k \in Keys : Admit(k)
    \/ \E k \in Keys : Backpressure(k)
    \/ Release
    \/ \E k \in Keys : Replay(k)

TypeOK ==
    /\ used \in Nat
    /\ used <= Capacity
    /\ accepted \subseteq Keys
    /\ retryable \subseteq Keys

CapacityBound ==
    used <= Capacity

RetryableIsNotAccepted ==
    accepted \cap retryable = {}

Spec == Init /\ [][Next]_vars

====

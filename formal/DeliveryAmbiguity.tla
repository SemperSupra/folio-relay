---- MODULE DeliveryAmbiguity ----

VARIABLES state, ambiguous, resolved

vars == <<state, ambiguous, resolved>>

States ==
    {"pending", "leased", "executing", "succeeded", "failed", "unknown", "cancelled"}

Init ==
    /\ state = "pending"
    /\ ambiguous = FALSE
    /\ resolved = FALSE

Lease ==
    /\ state = "pending"
    /\ state' = "leased"
    /\ UNCHANGED <<ambiguous, resolved>>

Execute ==
    /\ state = "leased"
    /\ state' = "executing"
    /\ UNCHANGED <<ambiguous, resolved>>

Succeed ==
    /\ state = "executing"
    /\ state' = "succeeded"
    /\ UNCHANGED <<ambiguous, resolved>>

FailDefinitive ==
    /\ state = "executing"
    /\ state' = "failed"
    /\ UNCHANGED <<ambiguous, resolved>>

BecomeUnknown ==
    /\ state = "executing"
    /\ state' = "unknown"
    /\ ambiguous' = TRUE
    /\ resolved' = FALSE

RetryFailed ==
    /\ state = "failed"
    /\ (~ambiguous \/ resolved)
    /\ state' = "pending"
    /\ UNCHANGED <<ambiguous, resolved>>

ResolveUnknownSucceeded ==
    /\ state = "unknown"
    /\ state' = "succeeded"
    /\ ambiguous' = TRUE
    /\ resolved' = TRUE

ResolveUnknownFailed ==
    /\ state = "unknown"
    /\ state' = "failed"
    /\ ambiguous' = TRUE
    /\ resolved' = TRUE

Cancel ==
    /\ state \in {"pending", "leased", "failed"}
    /\ state' = "cancelled"
    /\ UNCHANGED <<ambiguous, resolved>>

TerminalStutter ==
    /\ state \in {"succeeded", "cancelled"}
    /\ UNCHANGED vars

Next ==
    \/ Lease
    \/ Execute
    \/ Succeed
    \/ FailDefinitive
    \/ BecomeUnknown
    \/ RetryFailed
    \/ ResolveUnknownSucceeded
    \/ ResolveUnknownFailed
    \/ Cancel
    \/ TerminalStutter

TypeOK ==
    /\ state \in States
    /\ ambiguous \in BOOLEAN
    /\ resolved \in BOOLEAN

NoBlindRetryAfterAmbiguity ==
    ~(state = "pending" /\ ambiguous /\ ~resolved)

ResolutionImpliesAmbiguity ==
    resolved => ambiguous

Spec == Init /\ [][Next]_vars

====

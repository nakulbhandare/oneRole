package worker

import "github.com/oneRole/repository"

// These aliases keep orchestration coupled to the durable repository port and
// ensure the database and JetStream workers share the same fencing token.
type InboxClaim = repository.InboxClaim
type InboxClaimResult = repository.InboxClaimResult
type InboxStore = repository.InboxStore

const (
	InboxClaimAcquired    = repository.InboxClaimAcquired
	InboxAlreadyProcessed = repository.InboxAlreadyProcessed
	InboxLeaseHeld        = repository.InboxLeaseHeld
)

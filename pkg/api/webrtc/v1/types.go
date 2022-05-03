package v1

const (
	TypeGreeting = "greeting" // Greeting is a claim for a set of IDs
	TypeKick     = "kick"     // Kick notifies peers that an ID has already been claimed
	TypeBackoff  = "backoff"  // Backoff asks a peer to back off from claiming IDs
	TypeClaimed  = "claimed"  // Claimed notifies a peer that an ID has already been claimed
)

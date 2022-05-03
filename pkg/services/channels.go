package services

const (
	weronPrefix = "weron/" // General channel prefix

	EthernetPrimary = weronPrefix + "ethernet/primary" // Primary channel for Ethernet

	IPPrimary = weronPrefix + "ip/primary" // Primary channel for IP
	IPID      = weronPrefix + "ip/id"      // ID negotiation channel for IP

	ChatPrimary = weronPrefix + "chat/primary" // Primary channel for chat
	ChatID      = weronPrefix + "chat/id"      // ID negotiation channel for chat

	ThroughputPrimary = weronPrefix + "throughput/primary" // Primary channel for throughput measurements
	LatencyPrimary    = weronPrefix + "latency/primary"    // Primary channel for latency measurements

	IDGeneral = weronPrefix + "id/id" // General channel for ID negotiation
)

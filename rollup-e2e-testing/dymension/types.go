package dymension

type EibcEvent struct {
	ID           string `json:"id"`
	Price        string `json:"price"`
	Fee          string `json:"fee"`
	IsFulfilled  bool   `json:"is_fulfilled"`
	PacketStatus string `json:"packet_status"`
}

type RollappState struct {
	StateInfo StateInfo `json:"stateInfo"`
}

type StateInfo struct {
	StateInfoIndex   StateInfoIndex `json:"stateInfoIndex"`
	Sequencer        string         `json:"sequencer"`
	StartHeight      string         `json:"startHeight"`
	NumBlocks        string         `json:"numBlocks"`
	DAPath           string         `json:"DAPath"`
	Version          string         `json:"version"`
	CreationHeight   string         `json:"creationHeight"`
	Status           string         `json:"status"`
	BlockDescriptors BDs            `json:"BDs"`
}

type QueryGetRollappResponse struct {
	Rollapp                   Rollapp         `json:"rollapp"`
	LatestStateIndex          *StateInfoIndex `json:"latestStateIndex"`
	LatestFinalizedStateIndex *StateInfoIndex `json:"latestFinalizedStateIndex"`
}

type Rollapp struct {
	RollappId             string               `json:"rollappId"`
	Creator               string               `json:"creator"`
	Version               string               `json:"version"`
	CodeStamp             string               `json:"codeStamp"`            // Deprecated: Do not use.
	GenesisPath           string               `json:"genesisPath"`          // Deprecated: Do not use.
	MaxWithholdingBlocks  string               `json:"maxWithholdingBlocks"` // Deprecated: Do not use.
	MaxSequencers         string               `json:"maxSequencers"`
	PermissionedAddresses []string             `json:"permissionedAddresses"`
	TokenMetadata         []*TokenMetadata     `json:"tokenMetadata"`
	GenesisState          *RollappGenesisState `json:"genesis_state"`
	ChannelId             string               `json:"channel_id"`
	Frozen                bool                 `json:"frozen"`
}

type RollappGenesisState struct {
	GenesisAccounts []GenesisAccount `json:"genesis_accounts"`
	IsGenesisEvent  bool             `json:"is_genesis_event"`
}

type GenesisAccount struct {
	Amount  Coin   `json:"amount"`
	Address string `json:"address"`
}

type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type TokenMetadata struct {
	Description string       `json:"description"`
	DenomUnits  []*DenomUnit `json:"denom_units"`
	Base        string       `json:"base"`
	Display     string       `json:"display"`
	Name        string       `json:"name"`
	Symbol      string       `json:"symbol"`
	URI         string       `json:"uri"`
	URIHash     string       `json:"uri_hash"`
}

type DenomUnit struct {
	Denom    string   `json:"denom"`
	Exponent string   `json:"exponent"`
	Aliases  []string `json:"aliases"`
}

type StateInfoIndex struct {
	RollappId string `json:"rollappId"`
	Index     string `json:"index"`
}

type BDs struct {
	BD []BlockDescriptor `json:"BD"`
}

type BlockDescriptor struct {
	Height                 string `json:"height"`
	StateRoot              string `json:"stateRoot"`
	IntermediateStatesRoot string `json:"intermediateStatesRoot"`
}

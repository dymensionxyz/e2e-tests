package tests

import (
	"os"

	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/decentrio/rollup-e2e-testing/cosmos"
	"github.com/decentrio/rollup-e2e-testing/ibc"

	eibc "github.com/dymensionxyz/dymension/v3/x/eibc/types"
	ethermintcrypto "github.com/evmos/ethermint/crypto/codec"
	ethermint "github.com/evmos/ethermint/types"
)

var (
	DymensionMainRepo = "ghcr.io/dymensionxyz/dymension"

	RollappMainRepo = "ghcr.io/dymensionxyz/rollapp"

	dymensionVersion, rollappVersion = GetDockerImageVersion()

	dymensionImage = ibc.DockerImage{
		Repository: DymensionMainRepo,
		Version:    dymensionVersion,
		UidGid:     "1025:1025",
	}

	rollappImage = ibc.DockerImage{
		Repository: RollappMainRepo,
		Version:    rollappVersion,
		UidGid:     "1025:1025",
	}

	dymensionConfig = ibc.ChainConfig{
		Type:                "hub-dym",
		Name:                "dymension",
		ChainID:             "dymension_100-1",
		Images:              []ibc.DockerImage{dymensionImage},
		Bin:                 "dymd",
		Bech32Prefix:        "dym",
		Denom:               "udym",
		CoinType:            "118",
		GasPrices:           "0.0udym",
		EncodingConfig:      encodingConfig(),
		GasAdjustment:       1.1,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       nil,
		ConfigFileOverrides: nil,
	}
)

func GetDockerImageVersion() (dymensionVersion, rollappVersion string) {
	dymensionVersion, found := os.LookupEnv("DYMENSION_CI")
	if !found {
		dymensionVersion = "latest"
	}

	rollappVersion, found = os.LookupEnv("ROLLAPP_CI")
	if !found {
		rollappVersion = "latest"
	}
	return dymensionVersion, rollappVersion
}

func encodingConfig() *simappparams.EncodingConfig {
	cfg := cosmos.DefaultEncoding()

	ethermint.RegisterInterfaces(cfg.InterfaceRegistry)
	ethermintcrypto.RegisterInterfaces(cfg.InterfaceRegistry)
	eibc.RegisterInterfaces(cfg.InterfaceRegistry)
	return &cfg
}

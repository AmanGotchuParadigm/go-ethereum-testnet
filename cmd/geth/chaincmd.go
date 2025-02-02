// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	"github.com/urfave/cli/v2"
)

var (
	cliqueGenesisCommand = &cli.Command{
		Action: cliqueGenesis,
		Name:   "cliquegenesis",
		Usage:  "Change genesis state to clique consensus",
		Flags:  utils.DatabasePathFlags,
		Description: `
			the cliquegenesis command sets the genesis state to use clique consensus.
		`,
	}
	mainnetGenesisCommand = &cli.Command{
		Action: mainnetGenesis,
		Name:   "mainnetgenesis",
		Usage:  "Hard set genesis chain config to mainnet state.",
		Flags:  utils.DatabasePathFlags,
		Description: `
			the mainnetgenesis command hard sets the genesis configuration to a mainnet genesis state.
		`,
	}
	testnetGenesisCommand = &cli.Command{
		Action:    tesnetGenesis,
		Name:      "testnetgenesis",
		Usage:     "Hard set genesis chain config.",
		ArgsUsage: "<chainID>",
		Flags:     utils.DatabasePathFlags,
		Description: `
			the testnetgenesis command hard sets the genesis configuration to a testnet genesis state. 
		`,
	}
	mutateCommand = &cli.Command{
		Action:    mutateGenesis,
		Name:      "mutate",
		Usage:     "Change the genesis chain ID of a given data directory.",
		ArgsUsage: "<chainID>",
		Flags:     utils.DatabasePathFlags,
		Description: `
			The mutate command changes the chain ID of a given genesis block within a data directory. 
			This is a test to see if we can use data from a different chainID for a manually set one.
		`,
	}
	initCommand = &cli.Command{
		Action:    initGenesis,
		Name:      "init",
		Usage:     "Bootstrap and initialize a new genesis block",
		ArgsUsage: "<genesisPath>",
		Flags:     utils.DatabasePathFlags,
		Description: `
The init command initializes a new genesis block and definition for the network.
This is a destructive action and changes the network in which you will be
participating.

It expects the genesis file as argument.`,
	}
	dumpGenesisCommand = &cli.Command{
		Action:    dumpGenesis,
		Name:      "dumpgenesis",
		Usage:     "Dumps genesis block JSON configuration to stdout",
		ArgsUsage: "",
		Flags:     utils.NetworkFlags,
		Description: `
The dumpgenesis command dumps the genesis block configuration in JSON format to stdout.`,
	}
	importCommand = &cli.Command{
		Action:    importChain,
		Name:      "import",
		Usage:     "Import a blockchain file",
		ArgsUsage: "<filename> (<filename 2> ... <filename N>) ",
		Flags: flags.Merge([]cli.Flag{
			utils.CacheFlag,
			utils.SyncModeFlag,
			utils.GCModeFlag,
			utils.SnapshotFlag,
			utils.CacheDatabaseFlag,
			utils.CacheGCFlag,
			utils.MetricsEnabledFlag,
			utils.MetricsEnabledExpensiveFlag,
			utils.MetricsHTTPFlag,
			utils.MetricsPortFlag,
			utils.MetricsEnableInfluxDBFlag,
			utils.MetricsEnableInfluxDBV2Flag,
			utils.MetricsInfluxDBEndpointFlag,
			utils.MetricsInfluxDBDatabaseFlag,
			utils.MetricsInfluxDBUsernameFlag,
			utils.MetricsInfluxDBPasswordFlag,
			utils.MetricsInfluxDBTagsFlag,
			utils.MetricsInfluxDBTokenFlag,
			utils.MetricsInfluxDBBucketFlag,
			utils.MetricsInfluxDBOrganizationFlag,
			utils.TxLookupLimitFlag,
		}, utils.DatabasePathFlags),
		Description: `
The import command imports blocks from an RLP-encoded form. The form can be one file
with several RLP-encoded blocks, or several files can be used.

If only one file is used, import error will result in failure. If several files are used,
processing will proceed even if an individual RLP-file import failure occurs.`,
	}
	exportCommand = &cli.Command{
		Action:    exportChain,
		Name:      "export",
		Usage:     "Export blockchain into file",
		ArgsUsage: "<filename> [<blockNumFirst> <blockNumLast>]",
		Flags: flags.Merge([]cli.Flag{
			utils.CacheFlag,
			utils.SyncModeFlag,
		}, utils.DatabasePathFlags),
		Description: `
Requires a first argument of the file to write to.
Optional second and third arguments control the first and
last block to write. In this mode, the file will be appended
if already existing. If the file ends with .gz, the output will
be gzipped.`,
	}
	importPreimagesCommand = &cli.Command{
		Action:    importPreimages,
		Name:      "import-preimages",
		Usage:     "Import the preimage database from an RLP stream",
		ArgsUsage: "<datafile>",
		Flags: flags.Merge([]cli.Flag{
			utils.CacheFlag,
			utils.SyncModeFlag,
		}, utils.DatabasePathFlags),
		Description: `
The import-preimages command imports hash preimages from an RLP encoded stream.
It's deprecated, please use "geth db import" instead.
`,
	}
	exportPreimagesCommand = &cli.Command{
		Action:    exportPreimages,
		Name:      "export-preimages",
		Usage:     "Export the preimage database into an RLP stream",
		ArgsUsage: "<dumpfile>",
		Flags: flags.Merge([]cli.Flag{
			utils.CacheFlag,
			utils.SyncModeFlag,
		}, utils.DatabasePathFlags),
		Description: `
The export-preimages command exports hash preimages to an RLP encoded stream.
It's deprecated, please use "geth db export" instead.
`,
	}
	dumpCommand = &cli.Command{
		Action:    dump,
		Name:      "dump",
		Usage:     "Dump a specific block from storage",
		ArgsUsage: "[? <blockHash> | <blockNum>]",
		Flags: flags.Merge([]cli.Flag{
			utils.CacheFlag,
			utils.IterativeOutputFlag,
			utils.ExcludeCodeFlag,
			utils.ExcludeStorageFlag,
			utils.IncludeIncompletesFlag,
			utils.StartKeyFlag,
			utils.DumpLimitFlag,
		}, utils.DatabasePathFlags),
		Description: `
This command dumps out the state for a given block (or latest, if none provided).
`,
	}
)

func cliqueGenesis(ctx *cli.Context) error {
	// Loads geth configuration and creates a blank node instance.
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	// Open chain database
	chaindb, err := stack.OpenDatabaseWithFreezer("chaindata", 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
	if err != nil {
		utils.Fatalf("Failed to open database: %v", err)
	}

	/// Modify genesis chain config for clique

	// Get current genesis config
	genesisHash := rawdb.ReadCanonicalHash(chaindb, 0)
	genesisConfig := rawdb.ReadChainConfig(chaindb, genesisHash)

	// Add clique consensus data
	genesisConfig.Clique = &params.CliqueConfig{
		Period: 5,
		Epoch:  30000,
	}
	genesisConfig.Ethash = nil

	// Write genesis config
	rawdb.WriteChainConfig(chaindb, genesisHash, genesisConfig)

	// Read updated genesis config
	newConfig := rawdb.ReadChainConfig(chaindb, genesisHash)
	fmt.Println(newConfig)

	/// Modify genesis chain header for clique

	// Read existing header
	genesisHeaderNumber := rawdb.ReadHeaderNumber(chaindb, genesisHash)
	fmt.Println("Genesis header number", genesisHeaderNumber)
	genesisHeader := rawdb.ReadHeader(chaindb, genesisHash, *genesisHeaderNumber)
	fmt.Println("Genesis header hash", genesisHeader.Hash().String())

	// Add clique consensus data to genesisHeader
	genesisHeader.Number = big.NewInt(0)
	genesisHeader.Difficulty = big.NewInt(1)
	genesisHeader.GasLimit = 8000000

	extraHexData := `0000000000000000000000000000000000000000000000000000000000000000198400B2e59a28a62bbF1fAAAe6e33771d93Fa130000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000`
	data, err := hex.DecodeString(extraHexData)
	if err != nil {
		fmt.Println("Error decoding hex string", err)
		return err
	}
	genesisHeader.Extra = data

	// Writing updated header at hash
	rawdb.WriteHeader(chaindb, genesisHeader)

	// Fetch header by number to compare

	fmt.Println("Successfully overwrote chain config & header for clique consensus!")
	return nil
}

func mainnetGenesis(ctx *cli.Context) error {
	// Loads geth configuration and creates a blank node instance.
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()
	fmt.Println(stack)

	// Open chain database
	chaindb, err := stack.OpenDatabaseWithFreezer("chaindata", 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
	if err != nil {
		utils.Fatalf("Failed to open database: %v", err)
	}

	// Get current genesis config
	genesisHash := rawdb.ReadCanonicalHash(chaindb, 0)

	// Write genesis config
	rawdb.WriteChainConfig(chaindb, genesisHash, params.MainnetChainConfig)
	fmt.Println("Successfully overwrote chain config")

	// Read updated genesis config
	newConfig := rawdb.ReadChainConfig(chaindb, genesisHash)
	fmt.Println(newConfig)

	return nil
}

func tesnetGenesis(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		utils.Fatalf("need chainID as the only argument")
	}
	manualChainID := ctx.Args().First()
	fmt.Printf("Setting genesis chain ID to %s.\n", manualChainID)

	chainIDInt := new(big.Int)
	chainIDInt, ok := chainIDInt.SetString(manualChainID, 10)
	if !ok {
		fmt.Println("SetString: error")
		return nil
	}

	ParadigmChainConfig := &params.ChainConfig{
		ChainID:             chainIDInt,
		HomesteadBlock:      big.NewInt(1),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    nil,
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		ArrowGlacierBlock:   nil,
		Clique: &params.CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	// Loads geth configuration and creates a blank node instance.
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()
	fmt.Println(stack)

	// Open chain database
	chaindb, err := stack.OpenDatabaseWithFreezer("chaindata", 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
	if err != nil {
		utils.Fatalf("Failed to open database: %v", err)
	}

	// Get current genesis config
	genesisHash := rawdb.ReadCanonicalHash(chaindb, 0)

	// Write genesis config
	rawdb.WriteChainConfig(chaindb, genesisHash, ParadigmChainConfig)
	fmt.Println("Successfully overwrote chain config")

	// Read updated genesis config
	newConfig := rawdb.ReadChainConfig(chaindb, genesisHash)
	fmt.Println(newConfig)

	return nil
}

func mutateGenesis(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		utils.Fatalf("need chainID as the only argument")
	}
	manualChainID := ctx.Args().First()
	fmt.Printf("Setting genesis chain ID to %s.\n", manualChainID)

	// Loads geth configuration and creates a blank node instance.
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()
	fmt.Println(stack)

	// Open chain database
	chaindb, err := stack.OpenDatabaseWithFreezer("chaindata", 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
	if err != nil {
		utils.Fatalf("Failed to open database: %v", err)
	}

	// Get current genesis config
	genesisHash := rawdb.ReadCanonicalHash(chaindb, 0)
	storedConfig := rawdb.ReadChainConfig(chaindb, genesisHash)
	fmt.Println(storedConfig)

	// Change genesis config
	n := new(big.Int)
	n, ok := n.SetString(manualChainID, 10)
	if !ok {
		fmt.Println("SetString: error")
		return nil
	}
	storedConfig.ChainID = n

	// Write genesis config
	rawdb.WriteChainConfig(chaindb, genesisHash, storedConfig)
	fmt.Println("Successfully overwrote chain ID!")

	// Read updated genesis config
	newConfig := rawdb.ReadChainConfig(chaindb, genesisHash)
	fmt.Println(newConfig)

	return nil
}

// initGenesis will initialise the given JSON format genesis file and writes it as
// the zero'd block (i.e. genesis) or will fail hard if it can't succeed.
func initGenesis(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		utils.Fatalf("need genesis.json file as the only argument")
	}
	genesisPath := ctx.Args().First()
	if len(genesisPath) == 0 {
		utils.Fatalf("invalid path to genesis file")
	}
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}
	// Open and initialise both full and light databases
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()
	for _, name := range []string{"chaindata", "lightchaindata"} {
		chaindb, err := stack.OpenDatabaseWithFreezer(name, 0, 0, ctx.String(utils.AncientFlag.Name), "", false)
		if err != nil {
			utils.Fatalf("Failed to open database: %v", err)
		}
		_, hash, err := core.SetupGenesisBlock(chaindb, genesis)
		if err != nil {
			utils.Fatalf("Failed to write genesis block: %v", err)
		}
		chaindb.Close()
		log.Info("Successfully wrote genesis state", "database", name, "hash", hash)
	}
	return nil
}

func dumpGenesis(ctx *cli.Context) error {
	// TODO(rjl493456442) support loading from the custom datadir
	genesis := utils.MakeGenesis(ctx)
	if genesis == nil {
		genesis = core.DefaultGenesisBlock()
	}
	if err := json.NewEncoder(os.Stdout).Encode(genesis); err != nil {
		utils.Fatalf("could not encode genesis")
	}
	return nil
}

func importChain(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		utils.Fatalf("This command requires an argument.")
	}
	// Start metrics export if enabled
	utils.SetupMetrics(ctx)
	// Start system runtime metrics collection
	go metrics.CollectProcessMetrics(3 * time.Second)

	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	chain, db := utils.MakeChain(ctx, stack)
	defer db.Close()

	// Start periodically gathering memory profiles
	var peakMemAlloc, peakMemSys uint64
	go func() {
		stats := new(runtime.MemStats)
		for {
			runtime.ReadMemStats(stats)
			if atomic.LoadUint64(&peakMemAlloc) < stats.Alloc {
				atomic.StoreUint64(&peakMemAlloc, stats.Alloc)
			}
			if atomic.LoadUint64(&peakMemSys) < stats.Sys {
				atomic.StoreUint64(&peakMemSys, stats.Sys)
			}
			time.Sleep(5 * time.Second)
		}
	}()
	// Import the chain
	start := time.Now()

	var importErr error

	if ctx.Args().Len() == 1 {
		if err := utils.ImportChain(chain, ctx.Args().First()); err != nil {
			importErr = err
			log.Error("Import error", "err", err)
		}
	} else {
		for _, arg := range ctx.Args().Slice() {
			if err := utils.ImportChain(chain, arg); err != nil {
				importErr = err
				log.Error("Import error", "file", arg, "err", err)
			}
		}
	}
	chain.Stop()
	fmt.Printf("Import done in %v.\n\n", time.Since(start))

	// Output pre-compaction stats mostly to see the import trashing
	showLeveldbStats(db)

	// Print the memory statistics used by the importing
	mem := new(runtime.MemStats)
	runtime.ReadMemStats(mem)

	fmt.Printf("Object memory: %.3f MB current, %.3f MB peak\n", float64(mem.Alloc)/1024/1024, float64(atomic.LoadUint64(&peakMemAlloc))/1024/1024)
	fmt.Printf("System memory: %.3f MB current, %.3f MB peak\n", float64(mem.Sys)/1024/1024, float64(atomic.LoadUint64(&peakMemSys))/1024/1024)
	fmt.Printf("Allocations:   %.3f million\n", float64(mem.Mallocs)/1000000)
	fmt.Printf("GC pause:      %v\n\n", time.Duration(mem.PauseTotalNs))

	if ctx.Bool(utils.NoCompactionFlag.Name) {
		return nil
	}

	// Compact the entire database to more accurately measure disk io and print the stats
	start = time.Now()
	fmt.Println("Compacting entire database...")
	if err := db.Compact(nil, nil); err != nil {
		utils.Fatalf("Compaction failed: %v", err)
	}
	fmt.Printf("Compaction done in %v.\n\n", time.Since(start))

	showLeveldbStats(db)
	return importErr
}

func exportChain(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		utils.Fatalf("This command requires an argument.")
	}

	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	chain, _ := utils.MakeChain(ctx, stack)
	start := time.Now()

	var err error
	fp := ctx.Args().First()
	if ctx.Args().Len() < 3 {
		err = utils.ExportChain(chain, fp)
	} else {
		// This can be improved to allow for numbers larger than 9223372036854775807
		first, ferr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
		last, lerr := strconv.ParseInt(ctx.Args().Get(2), 10, 64)
		if ferr != nil || lerr != nil {
			utils.Fatalf("Export error in parsing parameters: block number not an integer\n")
		}
		if first < 0 || last < 0 {
			utils.Fatalf("Export error: block number must be greater than 0\n")
		}
		if head := chain.CurrentFastBlock(); uint64(last) > head.NumberU64() {
			utils.Fatalf("Export error: block number %d larger than head block %d\n", uint64(last), head.NumberU64())
		}
		err = utils.ExportAppendChain(chain, fp, uint64(first), uint64(last))
	}

	if err != nil {
		utils.Fatalf("Export error: %v\n", err)
	}
	fmt.Printf("Export done in %v\n", time.Since(start))
	return nil
}

// importPreimages imports preimage data from the specified file.
func importPreimages(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		utils.Fatalf("This command requires an argument.")
	}

	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	db := utils.MakeChainDatabase(ctx, stack, false)
	start := time.Now()

	if err := utils.ImportPreimages(db, ctx.Args().First()); err != nil {
		utils.Fatalf("Import error: %v\n", err)
	}
	fmt.Printf("Import done in %v\n", time.Since(start))
	return nil
}

// exportPreimages dumps the preimage data to specified json file in streaming way.
func exportPreimages(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		utils.Fatalf("This command requires an argument.")
	}
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	db := utils.MakeChainDatabase(ctx, stack, true)
	start := time.Now()

	if err := utils.ExportPreimages(db, ctx.Args().First()); err != nil {
		utils.Fatalf("Export error: %v\n", err)
	}
	fmt.Printf("Export done in %v\n", time.Since(start))
	return nil
}

func parseDumpConfig(ctx *cli.Context, stack *node.Node) (*state.DumpConfig, ethdb.Database, common.Hash, error) {
	db := utils.MakeChainDatabase(ctx, stack, true)
	var header *types.Header
	if ctx.NArg() > 1 {
		return nil, nil, common.Hash{}, fmt.Errorf("expected 1 argument (number or hash), got %d", ctx.NArg())
	}
	if ctx.NArg() == 1 {
		arg := ctx.Args().First()
		if hashish(arg) {
			hash := common.HexToHash(arg)
			if number := rawdb.ReadHeaderNumber(db, hash); number != nil {
				header = rawdb.ReadHeader(db, hash, *number)
			} else {
				return nil, nil, common.Hash{}, fmt.Errorf("block %x not found", hash)
			}
		} else {
			number, err := strconv.ParseUint(arg, 10, 64)
			if err != nil {
				return nil, nil, common.Hash{}, err
			}
			if hash := rawdb.ReadCanonicalHash(db, number); hash != (common.Hash{}) {
				header = rawdb.ReadHeader(db, hash, number)
			} else {
				return nil, nil, common.Hash{}, fmt.Errorf("header for block %d not found", number)
			}
		}
	} else {
		// Use latest
		header = rawdb.ReadHeadHeader(db)
	}
	if header == nil {
		return nil, nil, common.Hash{}, errors.New("no head block found")
	}
	startArg := common.FromHex(ctx.String(utils.StartKeyFlag.Name))
	var start common.Hash
	switch len(startArg) {
	case 0: // common.Hash
	case 32:
		start = common.BytesToHash(startArg)
	case 20:
		start = crypto.Keccak256Hash(startArg)
		log.Info("Converting start-address to hash", "address", common.BytesToAddress(startArg), "hash", start.Hex())
	default:
		return nil, nil, common.Hash{}, fmt.Errorf("invalid start argument: %x. 20 or 32 hex-encoded bytes required", startArg)
	}
	var conf = &state.DumpConfig{
		SkipCode:          ctx.Bool(utils.ExcludeCodeFlag.Name),
		SkipStorage:       ctx.Bool(utils.ExcludeStorageFlag.Name),
		OnlyWithAddresses: !ctx.Bool(utils.IncludeIncompletesFlag.Name),
		Start:             start.Bytes(),
		Max:               ctx.Uint64(utils.DumpLimitFlag.Name),
	}
	log.Info("State dump configured", "block", header.Number, "hash", header.Hash().Hex(),
		"skipcode", conf.SkipCode, "skipstorage", conf.SkipStorage,
		"start", hexutil.Encode(conf.Start), "limit", conf.Max)
	return conf, db, header.Root, nil
}

func dump(ctx *cli.Context) error {
	stack, _ := makeConfigNode(ctx)
	defer stack.Close()

	conf, db, root, err := parseDumpConfig(ctx, stack)
	if err != nil {
		return err
	}
	state, err := state.New(root, state.NewDatabase(db), nil)
	if err != nil {
		return err
	}
	if ctx.Bool(utils.IterativeOutputFlag.Name) {
		state.IterativeDump(conf, json.NewEncoder(os.Stdout))
	} else {
		if conf.OnlyWithAddresses {
			fmt.Fprintf(os.Stderr, "If you want to include accounts with missing preimages, you need iterative output, since"+
				" otherwise the accounts will overwrite each other in the resulting mapping.")
			return fmt.Errorf("incompatible options")
		}
		fmt.Println(string(state.Dump(conf)))
	}
	return nil
}

// hashish returns true for strings that look like hashes.
func hashish(x string) bool {
	_, err := strconv.Atoi(x)
	return err != nil
}

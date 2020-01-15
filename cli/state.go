package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"gopkg.in/urfave/cli.v2"
)

var stateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with and query filecoin chain state",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
	},
	Subcommands: []*cli.Command{
		statePowerCmd,
		stateSectorsCmd,
		stateProvingSetCmd,
		statePledgeCollateralCmd,
		stateListActorsCmd,
		stateListMinersCmd,
		stateGetActorCmd,
		stateLookupIDCmd,
		stateReplaySetCmd,
		stateSectorSizeCmd,
		stateReadStateCmd,
		stateListMessagesCmd,
		stateCallCmd,
	},
}

func parseTipSetString(cctx *cli.Context) ([]cid.Cid, error) {
	ts := cctx.String("tipset")
	if ts == "" {
		return nil, nil
	}

	strs := strings.Split(ts, ",")

	var cids []cid.Cid
	for _, s := range strs {
		c, err := cid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		cids = append(cids, c)
	}

	return cids, nil
}

func loadTipSet(ctx context.Context, cctx *cli.Context, api api.FullNode) (*types.TipSet, error) {
	cids, err := parseTipSetString(cctx)
	if err != nil {
		return nil, err
	}

	if len(cids) == 0 {
		return nil, nil
	}

	k := types.NewTipSetKey(cids...)
	ts, err := api.ChainGetTipSet(ctx, k)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

var statePowerCmd = &cli.Command{
	Name:  "power",
	Usage: "Query network or miner power",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var maddr address.Address
		if cctx.Args().Present() {
			maddr, err = address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		power, err := api.StateMinerPower(ctx, maddr, ts)
		if err != nil {
			return err
		}

		res := power.TotalPower
		if cctx.Args().Present() {
			res = power.MinerPower
		}

		fmt.Println(res.String())
		return nil
	},
}

var stateSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Query the sector set of a miner",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerSectors(ctx, maddr, ts)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateProvingSetCmd = &cli.Command{
	Name:  "proving",
	Usage: "Query the proving set of a miner",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerProvingSet(ctx, maddr, ts)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateReplaySetCmd = &cli.Command{
	Name:  "replay",
	Usage: "Replay a particular message within a tipset",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			fmt.Println("usage: <tipset> <message cid>")
			fmt.Println("The last cid passed will be used as the message CID")
			fmt.Println("All preceding ones will be used as the tipset")
			return nil
		}

		args := cctx.Args().Slice()
		mcid, err := cid.Decode(args[len(args)-1])
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		var tscids []cid.Cid
		for _, s := range args[:len(args)-1] {
			c, err := cid.Decode(s)
			if err != nil {
				return fmt.Errorf("tipset cid was invalid: %s", err)
			}
			tscids = append(tscids, c)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var headers []*types.BlockHeader
		for _, c := range tscids {
			h, err := api.ChainGetBlock(ctx, c)
			if err != nil {
				return err
			}

			headers = append(headers, h)
		}

		ts, err := types.NewTipSet(headers)
		if err != nil {
			return err
		}

		res, err := api.StateReplay(ctx, ts, mcid)
		if err != nil {
			return xerrors.Errorf("replay call failed: %w", err)
		}

		fmt.Println("Replay receipt:")
		fmt.Printf("Exit code: %d\n", res.Receipt.ExitCode)
		fmt.Printf("Return: %x\n", res.Receipt.Return)
		fmt.Printf("Gas Used: %s\n", res.Receipt.GasUsed)
		if res.Receipt.ExitCode != 0 {
			fmt.Printf("Error message: %q\n", res.Error)
		}

		return nil
	},
}

var statePledgeCollateralCmd = &cli.Command{
	Name:  "pledge-collateral",
	Usage: "Get minimum miner pledge collateral",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		coll, err := api.StatePledgeCollateral(ctx, ts)
		if err != nil {
			return err
		}

		fmt.Println(types.FIL(coll))
		return nil
	},
}

var stateListMinersCmd = &cli.Command{
	Name:  "list-miners",
	Usage: "list all miners in the network",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		miners, err := api.StateListMiners(ctx, ts)
		if err != nil {
			return err
		}

		for _, m := range miners {
			fmt.Println(m.String())
		}

		return nil
	},
}

var stateListActorsCmd = &cli.Command{
	Name:  "list-actors",
	Usage: "list all actors in the network",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		actors, err := api.StateListActors(ctx, ts)
		if err != nil {
			return err
		}

		for _, a := range actors {
			fmt.Println(a.String())
		}

		return nil
	},
}

var stateGetActorCmd = &cli.Command{
	Name:  "get-actor",
	Usage: "Print actor information",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateGetActor(ctx, addr, ts)
		if err != nil {
			return err
		}

		fmt.Printf("Address:\t%s\n", addr)
		fmt.Printf("Balance:\t%s\n", types.FIL(a.Balance))
		fmt.Printf("Nonce:\t\t%d\n", a.Nonce)
		fmt.Printf("Code:\t\t%s\n", a.Code)
		fmt.Printf("Head:\t\t%s\n", a.Head)

		return nil
	},
}

var stateLookupIDCmd = &cli.Command{
	Name:  "lookup",
	Usage: "Find corresponding ID address",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateLookupID(ctx, addr, ts)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", a)

		return nil
	},
}

var stateSectorSizeCmd = &cli.Command{
	Name:  "sector-size",
	Usage: "Look up miners sector size",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		ssize, err := api.StateMinerSectorSize(ctx, addr, ts)
		if err != nil {
			return err
		}

		fmt.Printf("%d\n", ssize)
		return nil
	},
}

var stateReadStateCmd = &cli.Command{
	Name:  "read-state",
	Usage: "View a json representation of an actors state",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, addr, ts)
		if err != nil {
			return err
		}

		as, err := api.StateReadState(ctx, act, ts)
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(as.State, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))

		return nil
	},
}

var stateListMessagesCmd = &cli.Command{
	Name:  "list-messages",
	Usage: "list messages on chain matching given criteria",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "to",
			Usage: "return messages to a given address",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "return messages from a given address",
		},
		&cli.Uint64Flag{
			Name:  "toheight",
			Usage: "don't look before given block height",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var toa, froma address.Address
		if tos := cctx.String("to"); tos != "" {
			a, err := address.NewFromString(tos)
			if err != nil {
				return fmt.Errorf("given 'to' address %q was invalid: %w", tos, err)
			}
			toa = a
		}

		if froms := cctx.String("from"); froms != "" {
			a, err := address.NewFromString(froms)
			if err != nil {
				return fmt.Errorf("given 'from' address %q was invalid: %w", froms, err)
			}
			froma = a
		}

		toh := cctx.Uint64("toheight")

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		msgs, err := api.StateListMessages(ctx, &types.Message{To: toa, From: froma}, ts, toh)
		if err != nil {
			return err
		}

		for _, c := range msgs {
			m, err := api.ChainGetMessage(ctx, c)
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
		}

		return nil
	},
}

var stateCallCmd = &cli.Command{
	Name:  "call",
	Usage: "Invoke a method on an actor locally",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "",
			Value: actors.NetworkAddress.String(),
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "specify value field for invocation",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "ret",
			Usage: "specify how to parse output (auto, raw, addr, big)",
			Value: "auto",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			return fmt.Errorf("must specify at least actor and method to invoke")
		}
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		toa, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("given 'to' address %q was invalid: %w", cctx.Args().First(), err)
		}

		froma, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return fmt.Errorf("given 'from' address %q was invalid: %w", cctx.String("from"), err)
		}

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		method, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("must pass method as a number")
		}

		value, err := types.ParseFIL(cctx.String("value"))
		if err != nil {
			return fmt.Errorf("failed to parse 'value': %s", err)
		}

		act, err := api.StateGetActor(ctx, toa, ts)
		if err != nil {
			return fmt.Errorf("failed to lookup target actor: %s", err)
		}

		params, err := parseParamsForMethod(act.Code, method, cctx.Args().Slice()[2:])
		if err != nil {
			return fmt.Errorf("failed to parse params: %s", err)
		}

		ret, err := api.StateCall(ctx, &types.Message{
			From:     froma,
			To:       toa,
			Value:    types.BigInt(value),
			GasLimit: types.NewInt(10000000000),
			GasPrice: types.NewInt(0),
			Method:   method,
			Params:   params,
		}, ts)
		if err != nil {
			return fmt.Errorf("state call failed: %s", err)
		}

		if ret.ExitCode != 0 {
			return fmt.Errorf("invocation failed (exit: %d): %s", ret.ExitCode, ret.Error)
		}

		s, err := formatOutput(cctx.String("ret"), ret.Return)
		if err != nil {
			return fmt.Errorf("failed to format output: %s", err)
		}

		fmt.Printf("return: %s\n", s)

		return nil
	},
}

func formatOutput(t string, val []byte) (string, error) {
	switch t {
	case "raw", "hex":
		return fmt.Sprintf("%x", val), nil
	case "address", "addr", "a":
		a, err := address.NewFromBytes(val)
		if err != nil {
			return "", err
		}
		return a.String(), nil
	case "big", "int", "bigint":
		bi := types.BigFromBytes(val)
		return bi.String(), nil
	case "fil":
		bi := types.FIL(types.BigFromBytes(val))
		return bi.String(), nil
	case "pid", "peerid", "peer":
		pid, err := peer.IDFromBytes(val)
		if err != nil {
			return "", err
		}

		return pid.Pretty(), nil
	case "auto":
		if len(val) == 0 {
			return "", nil
		}

		a, err := address.NewFromBytes(val)
		if err == nil {
			return "address: " + a.String(), nil
		}

		pid, err := peer.IDFromBytes(val)
		if err == nil {
			return "peerID: " + pid.Pretty(), nil
		}

		bi := types.BigFromBytes(val)
		return "bigint: " + bi.String(), nil
	default:
		return "", fmt.Errorf("unrecognized output type: %q", t)
	}
}

func parseParamsForMethod(act cid.Cid, method uint64, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, nil
	}

	var f interface{}
	switch act {
	case actors.StorageMarketCodeCid:
		f = actors.StorageMarketActor{}.Exports()[method]
	case actors.StorageMinerCodeCid:
		f = actors.StorageMinerActor{}.Exports()[method]
	case actors.StoragePowerCodeCid:
		f = actors.StoragePowerActor{}.Exports()[method]
	case actors.MultisigCodeCid:
		f = actors.MultiSigActor{}.Exports()[method]
	case actors.PaymentChannelCodeCid:
		f = actors.PaymentChannelActor{}.Exports()[method]
	default:
		return nil, fmt.Errorf("the lazy devs didnt add support for that actor to this call yet")
	}

	rf := reflect.TypeOf(f)
	if rf.NumIn() != 3 {
		return nil, fmt.Errorf("expected referenced method to have three arguments")
	}

	paramObj := rf.In(2).Elem()
	if paramObj.NumField() != len(args) {
		return nil, fmt.Errorf("not enough arguments given to call that method (expecting %d)", paramObj.NumField())
	}

	p := reflect.New(paramObj)
	for i := 0; i < len(args); i++ {
		switch paramObj.Field(i).Type {
		case reflect.TypeOf(address.Address{}):
			a, err := address.NewFromString(args[i])
			if err != nil {
				return nil, err
			}
			p.Elem().Field(i).Set(reflect.ValueOf(a))
		default:
			return nil, fmt.Errorf("unsupported type for call (TODO): %s", paramObj.Field(i).Type)
		}
	}

	m := p.Interface().(cbg.CBORMarshaler)
	buf := new(bytes.Buffer)
	if err := m.MarshalCBOR(buf); err != nil {
		return nil, fmt.Errorf("failed to marshal param object: %s", err)
	}
	return buf.Bytes(), nil
}

package main

import (
	"fmt"
	"net"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/urfave/cli/v2"
)

func ModbusListener(listenTo string, serverAddr string) {
	log.Info().Str("localaddr", listenTo).Msgf("Listening for connections")
	l, err := net.Listen("tcp", listenTo)
	if err != nil {
		log.Fatal().Msgf("%v", err)
	}
	defer l.Close()
	requests := make(chan ModbusRequest)
	go sender(requests, serverAddr)
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal().Msgf("%v", err)
		} else {
			addr := fmt.Sprintf("%v", conn.RemoteAddr())
			clog := log.With().Str("client", addr).Logger()
			clog.Info().Msg("Accepted connection")
			handleClient(conn, requests, clog)
		}
	}
}

func main() {

	app := &cli.App{
		Name:  "modbusproxy",
		Usage: "modbusproxy <server:port>\nCreates a proxy for a modbus server: modbus",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 2502,
				Usage: "port number to listen on",
			},
			&cli.StringFlag{
				Name:  "interface",
				Value: "",
				Usage: "interface to listen on",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Value: false,
				Usage: "debug logging",
			},
		},
		Action: func(cCtx *cli.Context) error {
			if cCtx.Args().Len() != 1 {
				cli.ShowAppHelp(cCtx)
				os.Exit(1)
			}
			server := cCtx.Args().Get(0)
			port := cCtx.Int("port")
			interf := cCtx.String("interface")
			listenTo := fmt.Sprintf("%s:%d", interf, port)
			debug := cCtx.Bool("debug")
			if debug {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			} else {
				zerolog.SetGlobalLevel(zerolog.InfoLevel)
			}
			ModbusListener(listenTo, server)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Msgf("%v", err)
	}
}

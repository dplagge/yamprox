/* This file is part of yamprox.

   Yamprox is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   yamprox is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with yamprox.  If not, see <http://www.gnu.org/licenses/>.
*/
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

func setDebug(debug bool) {
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func start(cCtx *cli.Context) error {
	if cCtx.Args().Len() != 1 {
		cli.ShowAppHelp(cCtx)
		os.Exit(1)
	}
	setDebug(cCtx.Bool("debug"))

	server := cCtx.Args().Get(0)
	port := cCtx.Int("port")
	interf := cCtx.String("interface")
	listenTo := fmt.Sprintf("%s:%d", interf, port)

	ModbusListener(listenTo, server)
	return nil
}

func main() {
	app := &cli.App{
		Name:  "yamprox",
		Usage: "yamprox <server:port>\nCreates a proxy for a modbus server.",
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
		Action: start,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Msgf("%v", err)
	}
}

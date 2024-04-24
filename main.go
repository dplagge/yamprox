package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/urfave/cli/v2"
)

type ModbusPDU struct {
	transaction uint16
	protocol    uint16
	unit        byte
	data        []byte
}

type ModbusRequest struct {
	pdu ModbusPDU
	rep chan ModbusPDU
}

type ReplyHandler struct {
	clientTransaction uint16
	rep               chan ModbusPDU
}

func sender(ch chan ModbusRequest, serverAddr string) {
	clog := log.With().Str("server", serverAddr).Logger()
	for {
		startServerLoop(ch, serverAddr, clog)
		log.Info().Msg("Sender connection closed, retrying")
		time.Sleep(1 * time.Second)
	}
}

func startServerLoop(ch chan ModbusRequest, serverAddr string, clog zerolog.Logger) {
	conn := connectToServer(serverAddr, clog)

	var mappings sync.Map
	go sendRequestsToServer(ch, conn, &mappings, clog)
	senderResponseHandler(conn, &mappings, clog)
}

func connectToServer(serverAddr string, clog zerolog.Logger) net.Conn {
	clog.Info().Msgf("Connecting to server")
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		clog.Fatal().Msgf("Error when looking up server: %v", err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		clog.Fatal().Msgf("Error connecting to server: %v", err)
	}
	return conn
}

func sendRequestsToServer(ch chan ModbusRequest, conn net.Conn, mappings *sync.Map, clog zerolog.Logger) {
	var nextTransactionId uint16 = 1
	for req := range ch {
		pdu := req.pdu
		writePdu(nextTransactionId, pdu, conn)
		clog.Debug().Uint16("clienttransaction", pdu.transaction).Uint16("servertransaction", nextTransactionId).Msg("Writing PDU to server")
		mappings.Store(nextTransactionId, ReplyHandler{pdu.transaction, req.rep})
		nextTransactionId += 1
	}
}

func senderResponseHandler(conn net.Conn, mappings *sync.Map, clog zerolog.Logger) {
	defer conn.Close()
	for {
		pdu, err := readPdu(conn, clog)
		if err != nil {
			clog.Error().Msgf("Error when reading response: %v", err)
			return
		}
		if entry, present := mappings.LoadAndDelete(pdu.transaction); present {
			rh := entry.(ReplyHandler)
			clog.Debug().Uint16("servertransaction", pdu.transaction).Uint16("clienttransaction", rh.clientTransaction).Int("datasize", len(pdu.data)).Msg("Read PDU from server")
			rh.rep <- pdu.replaceTransaction(rh.clientTransaction)
		} else {
			clog.Error().Msgf("Unexpected transaction %v, ignoring", pdu.transaction)
		}
	}
}

func (pdu ModbusPDU) replaceTransaction(newTransId uint16) ModbusPDU {
	return ModbusPDU{newTransId, pdu.protocol, pdu.unit, pdu.data}
}

func writePdu(transactionId uint16, pdu ModbusPDU, conn io.Writer) {
	packet := make([]byte, len(pdu.data)+7)
	binary.BigEndian.PutUint16(packet[0:2], transactionId)
	binary.BigEndian.PutUint16(packet[2:4], pdu.protocol)
	binary.BigEndian.PutUint16(packet[4:6], uint16(len(pdu.data)+1))
	packet[6] = pdu.unit

	for i, value := range pdu.data {
		packet[7+i] = value
	}

	conn.Write(packet)
}

func readPdu(conn io.Reader, clog zerolog.Logger) (pdu *ModbusPDU, err error) {
	header := make([]byte, 7)
	n, err := io.ReadAtLeast(conn, header, 7)
	if n < 7 {
		if err != io.EOF {
			if err != nil {
				clog.Error().Msgf("Error when reading header: %v", err)
			}
			err = errors.New("modbus header too short")
		}
		return
	}
	transaction := binary.BigEndian.Uint16(header[0:2])
	protocol := binary.BigEndian.Uint16(header[2:4])
	length := binary.BigEndian.Uint16(header[4:6])
	unit := header[6]
	// Just read lengh-1 bytes because the unit above was already the first byte
	data := make([]byte, length-1)
	n, err = io.ReadAtLeast(conn, data, int(length)-1)
	//log.Printf("Read %d data bytes", n)
	if n < int(length)-1 {
		clog.Error().Msgf("Error when reading header: %v", err)
		err = errors.New("modbus invalid data")
		return
	}
	pdu = &ModbusPDU{transaction, protocol, unit, data}
	err = nil
	return
}

func clientRequestHandler(conn net.Conn, responses chan ModbusPDU, toServer chan ModbusRequest, clog zerolog.Logger) {
	defer conn.Close()
	for {
		pdu, err := readPdu(conn, clog)
		if err != nil {
			if err == io.EOF {
				clog.Info().Msg("Client connection closed")
			} else {
				clog.Error().Msgf("Error when reading data from client: %v, closing connection", err)
			}
			return
		}
		clog.Debug().Uint16("clienttransaction", pdu.transaction).Int("datasize", len(pdu.data)).Msg("Received PDU from client")
		toServer <- ModbusRequest{*pdu, responses}
	}
}

func clientResponseHandler(conn io.Writer, fromServer chan ModbusPDU, clog zerolog.Logger) {
	for {
		pdu := <-fromServer
		clog.Debug().Uint16("clienttransaction", pdu.transaction).Msg("Writing response to client")
		writePdu(pdu.transaction, pdu, conn)
	}
}

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
			responses := make(chan ModbusPDU)
			go clientResponseHandler(conn, responses, clog)
			go clientRequestHandler(conn, responses, requests, clog)
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

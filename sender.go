package main

import (
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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

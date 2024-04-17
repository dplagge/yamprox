package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func sender(ch chan ModbusRequest) {
	serverAddr := "localhost:3333"
	clog := log.With().Str("server", serverAddr).Logger()
	clog.Info().Msgf("Connecting to server")
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		clog.Fatal().Msgf("Error when looking up server: %v", err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		clog.Fatal().Msgf("Error connecting to server: %v", err)
	}
	defer conn.Close()

	var mappings sync.Map
	go senderResponseHandler(conn, &mappings, clog)

	var nextTransactionId uint16 = 1
	for req := range ch {
		pdu := req.pdu
		writePdu(nextTransactionId, pdu, conn)
		mappings.Store(nextTransactionId, ReplyHandler{pdu.transaction, req.rep})
		nextTransactionId += 1
	}
}

func senderResponseHandler(conn net.Conn, mappings *sync.Map, clog zerolog.Logger) {
	for {
		pdu, err := readPdu(conn, clog)
		if err != nil {
			clog.Error().Msgf("Error when reading response: %v", err)
			return
		}
		if entry, present := mappings.LoadAndDelete(pdu.transaction); present {
			rh := entry.(ReplyHandler)
			clog.Info().Uint16("servertransaction", pdu.transaction).Uint16("clienttransaction", rh.clientTransaction).Int("datasize", len(pdu.data)).Msg("Read PDU from server")
			rh.rep <- pdu.replaceTransaction(rh.clientTransaction)
		} else {
			clog.Error().Msgf("Unexpected transaction %v, ignoring", pdu.transaction)
		}
	}
}

func (pdu ModbusPDU) replaceTransaction(newTransId uint16) ModbusPDU {
	return ModbusPDU{newTransId, -pdu.protocol, pdu.unit, pdu.data}
}

func writePdu(transactionId uint16, pdu ModbusPDU, conn io.Writer) {
	header := createPduHeader(transactionId, pdu)
	conn.Write(header)
	conn.Write(pdu.data)
}

func createPduHeader(transactionId uint16, pdu ModbusPDU) []byte {
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], transactionId)
	binary.BigEndian.PutUint16(header[2:4], pdu.protocol)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(pdu.data)+1))
	header[6] = pdu.unit
	return header
}

func readPdu(conn io.Reader, clog zerolog.Logger) (pdu *ModbusPDU, err error) {
	header := make([]byte, 7)
	n, err := io.ReadAtLeast(conn, header, 7)
	if n < 7 {
		if err != nil {
			clog.Error().Msgf("Error when reading header: %v", err)
		}
		err = errors.New("modbus header too short")
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
			clog.Error().Msgf("Error when reading data from client: %v", err)
			return
		}
		clog.Info().Uint16("clienttransaction", pdu.transaction).Int("datasize", len(pdu.data)).Msg("Received PDU from client")
		toServer <- ModbusRequest{*pdu, responses}
	}
}

func clientResponseHandler(conn io.Writer, fromServer chan ModbusPDU, clog zerolog.Logger) {
	for {
		pdu := <-fromServer
		clog.Info().Uint16("clienttransaction", pdu.transaction).Msg("Writing response to client")
		writePdu(pdu.transaction, pdu, conn)
	}
}

func ModbusListener() {
	l, err := net.Listen("tcp", ":2502")
	if err != nil {
		log.Fatal().Msgf("%v", err)
	}
	defer l.Close()
	requests := make(chan ModbusRequest)
	go sender(requests)
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
	ModbusListener()
}

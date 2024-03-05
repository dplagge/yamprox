package main

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
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
	serverAddr := "localhost:502"
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		log.Fatal("Error when looking up server %v: %v", serverAddr, err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Fatal("Error connecting to %v: %v", serverAddr, err)
	}
	defer conn.Close()

	var mappings sync.Map
	go senderResponseHandler(conn, &mappings)

	var nextTransactionId uint16 = 0
	for req := range ch {
		pdu := req.pdu
		writePdu(nextTransactionId, pdu, conn)
		mappings.Store(nextTransactionId, ReplyHandler{pdu.transaction, req.rep})
		nextTransactionId += 1
	}
}

func senderResponseHandler(conn io.Reader, mappings *sync.Map) {
	for {
		pdu, err := readPdu(conn)
		if err != nil {
			log.Printf("Error when reading response from server connection: %v", err)
			return
		}
		if entry, present := mappings.LoadAndDelete(pdu.transaction); present {
			rh := entry.(ReplyHandler)
			rh.rep <- pdu.replaceTransaction(rh.clientTransaction)
		} else {
			log.Printf("Unexpected transaction %v from server, ignoring", pdu.transaction)
		}
	}
}

func (pdu ModbusPDU) replaceTransaction(newTransId uint16) ModbusPDU {
	return ModbusPDU{newTransId, -pdu.protocol, pdu.unit, pdu.data}
}

func writePdu(transactionId uint16, pdu ModbusPDU, conn io.Writer) {
	header := createPdu(transactionId, pdu)
	conn.Write(header)
	conn.Write(pdu.data)
}

func createPdu(transactionId uint16, pdu ModbusPDU) []byte {
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], transactionId)
	binary.BigEndian.PutUint16(header[2:4], pdu.protocol)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(pdu.data)))
	header[6] = pdu.unit
	return header
}

func readPdu(conn io.Reader) (pdu *ModbusPDU, err error) {
	header := make([]byte, 7)
	n, err := conn.Read(header)
	if err != nil {
		log.Printf("Error when reading header from connection: %v", err)
		return
	}
	if n < 7 {
		err = errors.New("Modbus header too short")
	}
	transaction := binary.BigEndian.Uint16(header[0:2])
	protocol := binary.BigEndian.Uint16(header[2:4])
	length := binary.BigEndian.Uint16(header[4:6])
	unit := header[6]
	data := make([]byte, length)
	n, err = conn.Read(data)
	if err == nil {
		pdu = &ModbusPDU{transaction, protocol, unit, data}
	}
	return
}

func clientRequestHandler(conn net.Conn, responses chan ModbusPDU, toServer chan ModbusRequest) {
	defer conn.Close()
	for {
		pdu, err := readPdu(conn)
		if err != nil {
			log.Printf("Error when reading data from client: %v", err)
			return
		}
		toServer <- ModbusRequest{*pdu, responses}
	}
}

func clientResponseHandler(conn io.Writer, fromServer chan ModbusPDU) {
	for {
		pdu := <-fromServer
		writePdu(pdu.transaction, pdu, conn)
	}
}

func ModbusListener() {
	l, err := net.Listen("tcp", ":2502")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	requests := make(chan ModbusRequest)
	go sender(requests)
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		} else {
			log.Printf("Accepted connection from %v", conn.RemoteAddr())
			responses := make(chan ModbusPDU)
			go clientResponseHandler(conn, responses)
			go clientRequestHandler(conn, responses, requests)
		}
	}
}

func main() {
	ModbusListener()
}

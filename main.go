package main

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	serverAddr := "localhost:3333"
	log.Printf("Connecting to %v", serverAddr)
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		log.Fatalf("Error when looking up server %v: %v", serverAddr, err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Fatalf("Error connecting to %v: %v", serverAddr, err)
	}
	defer conn.Close()

	var mappings sync.Map
	go senderResponseHandler(conn, &mappings)

	var nextTransactionId uint16 = 1
	for req := range ch {
		pdu := req.pdu
		writePdu(nextTransactionId, pdu, conn)
		mappings.Store(nextTransactionId, ReplyHandler{pdu.transaction, req.rep})
		nextTransactionId += 1
	}
}

func senderResponseHandler(conn net.Conn, mappings *sync.Map) {
	for {
		name := fmt.Sprintf("server %v", conn.RemoteAddr())
		pdu, err := readPdu(name, conn)
		if err != nil {
			log.Printf("Error when reading response from %s: %v", name, err)
			return
		}
		//log.Printf("Read PDU from server")
		if entry, present := mappings.LoadAndDelete(pdu.transaction); present {
			rh := entry.(ReplyHandler)
			log.Printf("Read PDU from %s, server transaction=%d, client transaction=%d, data size=%d", name, pdu.transaction, rh.clientTransaction, len(pdu.data))
			rh.rep <- pdu.replaceTransaction(rh.clientTransaction)
		} else {
			log.Printf("Unexpected transaction %v from %s, ignoring", pdu.transaction, name)
		}
	}
}

func (pdu ModbusPDU) replaceTransaction(newTransId uint16) ModbusPDU {
	return ModbusPDU{newTransId, -pdu.protocol, pdu.unit, pdu.data}
}

func writePdu(transactionId uint16, pdu ModbusPDU, conn io.Writer) {
	header := createPduHeader(transactionId, pdu)
	conn.Write(header)
	//log.Printf("Wrote header, header length=%d", len(header))
	conn.Write(pdu.data)
	//log.Printf("Wrote data (data length=%d)", len(pdu.data))
}

func createPduHeader(transactionId uint16, pdu ModbusPDU) []byte {
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], transactionId)
	binary.BigEndian.PutUint16(header[2:4], pdu.protocol)
	binary.BigEndian.PutUint16(header[4:6], uint16(len(pdu.data)+1))
	header[6] = pdu.unit
	return header
}

func readPdu(name string, conn io.Reader) (pdu *ModbusPDU, err error) {
	header := make([]byte, 7)
	n, err := io.ReadAtLeast(conn, header, 7)
	if n < 7 {
		if err != nil {
			log.Printf("Error when reading header from connection %s: %v", name, err)
		}
		err = errors.New("Modbus header too short")
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
		log.Printf("Error when reading header from connection %s: %v", name, err)
		err = errors.New("Modbus invalid data")
		return
	}
	pdu = &ModbusPDU{transaction, protocol, unit, data}
	err = nil
	log.Printf("Read PDU from %s, transaction=%d, data size=%d", name, transaction, len(data))
	return
}

func clientRequestHandler(conn net.Conn, responses chan ModbusPDU, toServer chan ModbusRequest) {
	defer conn.Close()
	for {
		name := fmt.Sprintf("client %v", conn.RemoteAddr())
		pdu, err := readPdu(name, conn)
		if err != nil {
			log.Printf("Error when reading data from client: %v", err)
			return
		}
		toServer <- ModbusRequest{*pdu, responses}
	}
}

func clientResponseHandler(conn net.Conn, fromServer chan ModbusPDU) {
	for {
		pdu := <-fromServer
		log.Printf("Writing response to client %v, transaction=%d", conn.RemoteAddr(), pdu.transaction)
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

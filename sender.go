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
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ReplyHandler struct {
	clientTransaction uint16
	rep               chan ModbusPDU
	requestTime       time.Time
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

	// The mappings are used to map transaction IDs for the proxy-server connection to
	// 1. the transaction IDs of the client-proxy connection,
	// 2. the channel to write the responses to be picked up by the client response handler.
	// Since the client specifies the ID, we could have conflicts otherwise.

	var mappings sync.Map
	// We have one routine to take the requests from the channel ch and write them to the server.
	// That routine gives the request a new transaction ID and stores the mapping to the
	// client transaction ID in the map.
	go sendRequestsToServer(ch, conn, &mappings, clog)
	// Start a loop that removes old requests in case they do not get answered
	go cleanupDanglingRequests(&mappings, clog)
	// The response handler receives a
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
		// Store a mapping from the transaction ID to the client transaction ID and the channel for the responses
		mappings.Store(nextTransactionId, ReplyHandler{pdu.transaction, req.rep, time.Now()})
		// Write client request to server, but with own transaction ID
		writePdu(nextTransactionId, pdu, conn)
		clog.Debug().Uint16("clienttransaction", pdu.transaction).Uint16("servertransaction", nextTransactionId).Msg("Writing PDU to server")
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
		// Map the transaction ID of the server response to the transaction ID of the
		// client request and the chanel for the client response handler
		if entry, present := mappings.LoadAndDelete(pdu.transaction); present {
			rh := entry.(ReplyHandler)
			clog.Debug().Uint16("servertransaction", pdu.transaction).Uint16("clienttransaction", rh.clientTransaction).Int("datasize", len(pdu.data)).Msg("Read PDU from server")
			// Replace transaction  in PDU and write it to client queue
			rh.rep <- pdu.replaceTransaction(rh.clientTransaction)
		} else {
			clog.Error().Msgf("Unexpected transaction %v, ignoring", pdu.transaction)
		}
	}
}

func (pdu ModbusPDU) replaceTransaction(newTransId uint16) ModbusPDU {
	return ModbusPDU{newTransId, pdu.protocol, pdu.unit, pdu.data}
}

func cleanupDanglingRequests(mappings *sync.Map, clog zerolog.Logger) {
	for t := range time.Tick(time.Minute * 1) {
		lastRequestTime := t.Add(time.Duration(-5) * time.Minute)
		mappings.Range(func(key any, value any) bool {
			rh := value.(ReplyHandler)
			if rh.requestTime.Before(lastRequestTime) {
				mappings.Delete(key)
				clog.Info().Msg("Deleted unanswered request (5 minutes)")
			}
			return true
		})
	}
}

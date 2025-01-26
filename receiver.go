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
	"io"
	"net"

	"github.com/rs/zerolog"
)

// Handles a client connection by starting two routines, one for listening to
// client requests, the other for sending server responses to the client.
// The request is sent to the server via the given channel (see sendRequestsToServer)
func handleClient(conn net.Conn, toServer chan ModbusRequest, clog zerolog.Logger) {
	// This channel serves for sending incoming responses from the server to
	// the handler for sending them to the client.
	responses := make(chan ModbusPDU)

	// Start the routine for sending responses from the server to the client
	go clientResponseHandler(conn, responses, clog)
	// Start the routine for accepting requests and sending them to the sender component
	go clientRequestHandler(conn, responses, toServer, clog)
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
		// Just send the request together with the channel for the responses to the sender component
		// The sender will then use the given channel to route responses back to the client response handler.
		toServer <- ModbusRequest{*pdu, responses}
	}
}

func clientResponseHandler(conn io.Writer, fromServer chan ModbusPDU, clog zerolog.Logger) {
	for {
		// A PDU comes from the server, we just write it back into the client connection
		pdu := <-fromServer
		clog.Debug().Uint16("clienttransaction", pdu.transaction).Msg("Writing response to client")
		writePdu(pdu.transaction, pdu, conn)
	}
}

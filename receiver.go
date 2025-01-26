package main

import (
	"io"
	"net"

	"github.com/rs/zerolog"
)

func handleClient(conn net.Conn, toServer chan ModbusRequest, clog zerolog.Logger) {
	responses := make(chan ModbusPDU)

	go clientResponseHandler(conn, responses, clog)
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

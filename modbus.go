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
	"encoding/binary"
	"errors"
	"io"

	"github.com/rs/zerolog"
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

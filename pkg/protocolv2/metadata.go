// Copyright (C) 2023  mieru authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package protocolv2

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/enfein/mieru/pkg/mathext"
)

const (

	// Number of bytes used by metadata before encryption.
	metadataLength = 32

	// Protocols.
	closeConnRequest     byte = 0
	closeConnResponse    byte = 1
	openSessionRequest   byte = 2
	openSessionResponse  byte = 3
	closeSessionRequest  byte = 4
	closeSessionResponse byte = 5
	dataClientToServer   byte = 6
	dataServerToClient   byte = 7
	ackClientToServer    byte = 8
	ackServerToClient    byte = 9
)

// metadata defines the methods supported by all metadata.
type metadata interface {

	// Protocol returns the protocol of metadata.
	Protocol() byte

	// Marshal serializes the metadata to a non-encrypted wire format.
	Marshal() ([]byte, error)

	// Unmarshal constructs the metadata from the non-encrypted wire format.
	Unmarshal([]byte) error
}

var (
	_ metadata = &sessionStruct{}
	_ metadata = &dataAckStruct{}
)

// baseStruct is shared by all metadata struct.
type baseStruct struct {
	protocol  uint8  // byte 0: protocol type
	epoch     uint8  // byte 1: epoch, it changes when the underlay address is changed
	timestamp uint32 // byte 2 - 5: timestamp, number of minutes after UNIX epoch
}

// sessionStruct is used to open or close a session.
type sessionStruct struct {
	baseStruct
	sessionID  uint32 // byte 6 - 9: session ID number
	statusCode uint8  // byte 10: status of opening or closing session
	payloadLen uint16 // byte 11 - 12: length of encapsulated payload, not including auth tag
	suffixLen  uint8  // byte 13: length of suffix padding
}

func (ss *sessionStruct) Protocol() byte {
	return ss.baseStruct.protocol
}

func (ss *sessionStruct) Marshal() ([]byte, error) {
	b := make([]byte, metadataLength)
	b[0] = ss.baseStruct.protocol
	b[1] = ss.baseStruct.epoch
	ss.baseStruct.timestamp = uint32(time.Now().Unix() / 60)
	binary.BigEndian.PutUint32(b[2:], ss.baseStruct.timestamp)
	binary.BigEndian.PutUint32(b[6:], ss.sessionID)
	b[10] = ss.statusCode
	binary.BigEndian.PutUint16(b[11:], ss.payloadLen)
	b[13] = ss.suffixLen
	return b, nil
}

func (ss *sessionStruct) Unmarshal(b []byte) error {
	// Check errors.
	if len(b) != metadataLength {
		return fmt.Errorf("input bytes: %d, want %d", len(b), metadataLength)
	}
	if b[0] != openSessionRequest && b[0] != openSessionResponse && b[0] != closeSessionRequest && b[0] != closeSessionResponse {
		return fmt.Errorf("invalid protocol %d", b[0])
	}
	originalTimestamp := binary.BigEndian.Uint32(b[2:])
	currentTimestamp := uint32(time.Now().Unix() / 60)
	if !mathext.WithinRange(currentTimestamp, originalTimestamp, 1) {
		return fmt.Errorf("invalid timestamp %d", originalTimestamp*60)
	}

	// Do unmarshal.
	ss.baseStruct.protocol = b[0]
	ss.baseStruct.epoch = b[1]
	ss.baseStruct.timestamp = originalTimestamp
	ss.sessionID = binary.BigEndian.Uint32(b[6:])
	ss.statusCode = b[10]
	ss.payloadLen = binary.BigEndian.Uint16(b[11:])
	ss.suffixLen = b[13]
	return nil
}

func isSessionProtocol(p byte) bool {
	return p == openSessionRequest || p == openSessionResponse || p == closeSessionRequest || p == closeSessionResponse
}

func toSessionStruct(m metadata) (*sessionStruct, bool) {
	if isSessionProtocol(m.Protocol()) {
		return m.(*sessionStruct), true
	}
	return nil, false
}

// dataAckStruct is used by data and ack protocols.
type dataAckStruct struct {
	baseStruct
	sessionID  uint32 // byte 6 - 9: session ID number
	seq        uint32 // byte 10 - 13: sequence number or ack number
	unAckSeq   uint32 // byte 14 - 17: unacknowledged sequence number -- sequences smaller than this are acknowledged
	windowSize uint16 // byte 18 - 19: receive window size, in number of segments
	fragment   uint8  // byte 20: fragment number, 0 is the last fragment
	prefixLen  uint8  // byte 21: length of prefix padding
	payloadLen uint16 // byte 22 - 23: length of encapsulated payload, not including auth tag
	suffixLen  uint8  // byte 24: length of suffix padding
}

func (das *dataAckStruct) Protocol() byte {
	return das.baseStruct.protocol
}

func (das *dataAckStruct) Marshal() ([]byte, error) {
	b := make([]byte, metadataLength)
	b[0] = das.baseStruct.protocol
	b[1] = das.baseStruct.epoch
	das.baseStruct.timestamp = uint32(time.Now().Unix() / 60)
	binary.BigEndian.PutUint32(b[2:], das.baseStruct.timestamp)
	binary.BigEndian.PutUint32(b[6:], das.sessionID)
	binary.BigEndian.PutUint32(b[10:], das.seq)
	binary.BigEndian.PutUint32(b[14:], das.unAckSeq)
	binary.BigEndian.PutUint16(b[18:], das.windowSize)
	b[20] = das.fragment
	b[21] = das.prefixLen
	binary.BigEndian.PutUint16(b[22:], das.payloadLen)
	b[24] = das.suffixLen
	return b, nil
}

func (das *dataAckStruct) Unmarshal(b []byte) error {
	// Check errors.
	if len(b) != metadataLength {
		return fmt.Errorf("input bytes: %d, want %d", len(b), metadataLength)
	}
	if b[0] != dataClientToServer && b[0] != dataServerToClient && b[0] != ackClientToServer && b[0] != ackServerToClient {
		return fmt.Errorf("invalid protocol %d", b[0])
	}
	originalTimestamp := binary.BigEndian.Uint32(b[2:])
	currentTimestamp := uint32(time.Now().Unix() / 60)
	if !mathext.WithinRange(currentTimestamp, originalTimestamp, 1) {
		return fmt.Errorf("invalid timestamp %d", originalTimestamp*60)
	}

	// Do unmarshal.
	das.baseStruct.protocol = b[0]
	das.baseStruct.epoch = b[1]
	das.baseStruct.timestamp = originalTimestamp
	das.sessionID = binary.BigEndian.Uint32(b[6:])
	das.seq = binary.BigEndian.Uint32(b[10:])
	das.unAckSeq = binary.BigEndian.Uint32(b[14:])
	das.windowSize = binary.BigEndian.Uint16(b[18:])
	das.fragment = b[20]
	das.prefixLen = b[21]
	das.payloadLen = binary.BigEndian.Uint16(b[22:])
	das.suffixLen = b[24]
	return nil
}

func isDataAckProtocol(p byte) bool {
	return p == dataClientToServer || p == dataServerToClient || p == ackClientToServer || p == ackServerToClient
}

func toDataAckStruct(m metadata) (*dataAckStruct, bool) {
	if isDataAckProtocol(m.Protocol()) {
		return m.(*dataAckStruct), true
	}
	return nil, false
}

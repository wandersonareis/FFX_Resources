package event

import (
	"bytes"
	"encoding/binary"
	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	"fmt"
)

type FieldString struct {
	Charset           string
	Version           common.GameVersion
	RegularOffset     int
	RegularFlags      int
	RegularChoices    int
	SimplifiedOffset  int
	SimplifiedFlags   int
	SimplifiedChoices int
	RegularBytes      []byte
	SimplifiedBytes   []byte
	RegularString     string
	SimplifiedString  string
}

func NewEmptyFieldString(charset string, version common.GameVersion) *FieldString {
	return &FieldString{
		Charset: charset,
		Version: version,
	}
}

func NewFieldString(charset string, textOffset uint16, textFlags uint8, textChoices uint8, simplifiedTextOffset uint16, simplifiedTextFlags uint8, simplifiedTextChoices uint8, stringBytes []byte, version common.GameVersion) *FieldString {
	fs := &FieldString{
		Charset:           charset,
		Version:           version,
		RegularOffset:     int(textOffset),
		RegularFlags:      int(textFlags),
		RegularChoices:    int(textChoices),
		SimplifiedOffset:  int(simplifiedTextOffset),
		SimplifiedFlags:   int(simplifiedTextFlags),
		SimplifiedChoices: int(simplifiedTextChoices),
	}

	fs.RegularBytes = converter.GetStringBytesAtLookupOffset(stringBytes, fs.RegularOffset)
	fs.RegularString = converter.BytesToString(fs.RegularBytes, fs.Charset, fs.Version)

	if fs.RegularOffset == fs.SimplifiedOffset {
		fs.SimplifiedBytes = fs.RegularBytes
	} else {
		fs.SimplifiedBytes = converter.GetStringBytesAtLookupOffset(stringBytes, fs.SimplifiedOffset)
		fs.SimplifiedString = converter.BytesToString(fs.SimplifiedBytes, fs.Charset, fs.Version)
	}

	return fs
}

func FromFieldStringData(bytes []byte, charset string, version common.GameVersion) ([]*FieldString, error) {
	if len(bytes) == 0 {
		return []*FieldString{}, nil
	}
	if len(bytes) < 2 {
		return nil, fmt.Errorf("event string buffer too short")
	}

	first := binary.LittleEndian.Uint16(bytes)
	if first%8 != 0 || first < 8 {
		return nil, fmt.Errorf("first=%d invalid event string header", first)
	}
	if int(first) > len(bytes) {
		return nil, fmt.Errorf("event string header size %d exceeds buffer length %d", first, len(bytes))
	}

	count := int(first / 8)
	strings := make([]*FieldString, 0, count)

	for i := range count {
		offset := i * 8
		fieldString := NewFieldString(
			charset,
			binary.LittleEndian.Uint16(bytes[offset:offset+2]),
			bytes[offset+2],
			bytes[offset+3],
			binary.LittleEndian.Uint16(bytes[offset+4:offset+6]),
			bytes[offset+6],
			bytes[offset+7],
			bytes,
			version,
		)
		strings = append(strings, fieldString)
	}

	return strings, nil
}

func RebuildFieldStrings(strings []*FieldString, charset string, version common.GameVersion) []byte {
	count := len(strings)
	contentOffset := count * 8
	offsetMap := make(map[string]int)
	var buf bytes.Buffer

	for _, fieldString := range strings {
		regularString := fieldString.GetRegularString()
		fieldString.RegularChoices = converter.GetChoicesInString(regularString)

		// Caminho unico do texto: o FillByteList grava conteudo E terminador
		// (vazio = so o terminador, como no original). O dedupe por conteudo
		// so emite bytes na PRIMEIRA ocorrencia — no hit o offset reaproveita
		// a string ja gravada. Dedupe nao cobre o vazio: todas as linhas
		// vazias apontariam para o MESMO terminador, e apontar para
		// contentOffset aliasaria a primeira string.
		if offset, exists := offsetMap[regularString]; regularString != "" && exists {
			fieldString.RegularOffset = contentOffset + offset
		} else {
			fieldString.RegularOffset = contentOffset + buf.Len()
			if regularString != "" {
				offsetMap[regularString] = buf.Len()
			}
			converter.FillByteList(regularString, &buf, charset, version)
		}

		simplifiedString := fieldString.GetSimplifiedString()
		fieldString.SimplifiedChoices = converter.GetChoicesInString(simplifiedString)

		// Os dois lados vazios: um unico terminador serve os dois offsets (o
		// original grava os dois iguais) — sem o FillByteList do simplified.
		if regularString == "" && simplifiedString == "" {
			fieldString.SimplifiedOffset = fieldString.RegularOffset
			continue
		}

		if offset, exists := offsetMap[simplifiedString]; simplifiedString != "" && exists {
			fieldString.SimplifiedOffset = contentOffset + offset
		} else {
			fieldString.SimplifiedOffset = contentOffset + buf.Len()
			if simplifiedString != "" {
				offsetMap[simplifiedString] = buf.Len()
			}
			converter.FillByteList(simplifiedString, &buf, charset, version)
		}
	}

	return buf.Bytes()
}

func (fs *FieldString) ToRegularHeaderBytes() []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(
		fs.RegularOffset|fs.RegularFlags<<16|fs.RegularChoices<<24))
	return buf
}

func (fs *FieldString) ToSimplifiedHeaderBytes() []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(
		fs.SimplifiedOffset|fs.SimplifiedFlags<<16|fs.SimplifiedChoices<<24))
	return buf
}

func (fs *FieldString) String() string {
	if fs.HasDistinctSimplified() {
		return fs.GetRegularString() + " (Simplified: " + fs.GetSimplifiedString() + ")"
	}
	return fs.GetRegularString()
}

func (fs *FieldString) IsEmpty() bool {
	return fs.GetRegularString() == "" && fs.GetSimplifiedString() == ""
}

func (fs *FieldString) GetRegularString() string {
	return converter.BytesToString(fs.RegularBytes, fs.Charset, fs.Version)
}

func (fs *FieldString) GetSimplifiedString() string {
	return converter.BytesToString(fs.SimplifiedBytes, fs.Charset, fs.Version)
}

func (fs *FieldString) HasDistinctSimplified() bool {
	if len(fs.RegularBytes) != len(fs.SimplifiedBytes) {
		return true
	}
	for i, b := range fs.RegularBytes {
		if b != fs.SimplifiedBytes[i] {
			return true
		}
	}
	return false
}

func (fs *FieldString) SetRegularString(str string, newCharset ...string) {
	if len(newCharset) > 0 && newCharset[0] != "" {
		fs.SetCharset(newCharset[0])
	}

	keepSimplifiedSynced := !fs.HasDistinctSimplified()
	encoded, err := converter.StringToBytes(str, fs.Charset, fs.Version)
	if err != nil {
		common.LogError("SetRegularString: %v", err)
		return
	}
	fs.RegularBytes = encoded

	if keepSimplifiedSynced {
		fs.SimplifiedBytes = fs.RegularBytes
	}
}

func (fs *FieldString) SetSimplifiedString(str string, newCharset ...string) {
	if len(newCharset) > 0 && newCharset[0] != "" {
		fs.SetCharset(newCharset[0])
	}

	encoded, err := converter.StringToBytes(str, fs.Charset, fs.Version)
	if err != nil {
		common.LogError("SetSimplifiedString: %v", err)
		return
	}
	fs.SimplifiedBytes = encoded
}

func (fs *FieldString) SetCharset(newCharset string) {
	if newCharset != "" && newCharset != fs.Charset {
		fs.Charset = newCharset
	}
}

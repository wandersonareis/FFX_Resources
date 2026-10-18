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

// RebuildFieldStrings devolve o CONTEUDO da tabela (a payload depois dos
// cabecalhos). O layout (field_layout.go) decide o que sai e em que ordem —
// dedupe de armazenamento e linha vazia com terminador proprio — e a
// conversao string para bytes fica so com o converter. Os offsets gravados
// nas linhas sao os que a montagem do binario le para escrever cabecalhos.
func RebuildFieldStrings(strings []*FieldString, charset string, version common.GameVersion) []byte {
	count := len(strings)
	contentOffset := count * 8

	regular := make([]string, len(strings))
	simplified := make([]string, len(strings))
	for i, fs := range strings {
		regular[i] = fs.GetRegularString()
		simplified[i] = fs.GetSimplifiedString()
		fs.RegularChoices = converter.GetChoicesInString(regular[i])
		fs.SimplifiedChoices = converter.GetChoicesInString(simplified[i])
	}

	// O tamanho em bytes so existe apos a gravacao: o layout devolve indices
	// de emissao e os offsets nascem do inicio de cada emissao no buffer.
	reg, simp, emit := fieldLayout(regular, simplified)

	var buf bytes.Buffer
	starts := make([]int, len(emit))
	for i, t := range emit {
		starts[i] = buf.Len()
		converter.FillByteList(t, &buf, charset, version)
	}
	for i, fs := range strings {
		fs.RegularOffset = contentOffset + starts[reg[i]]
		fs.SimplifiedOffset = contentOffset + starts[simp[i]]
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

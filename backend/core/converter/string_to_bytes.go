package converter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"ffxresources/backend/common"
	encoding "ffxresources/backend/core/encoding"
)

var (
	WriteLinebreaksAsCommands = true
)

var (
	reCmd    = regexp.MustCompile(`^CMD:([0-9A-Fa-f]{1,2}):([0-9A-Fa-f]{1,2})`)
	reChoice = regexp.MustCompile(`\{CHOICE:([0-9A-Fa-f]{2})\}`)
	reMCR    = regexp.MustCompile(`^MCR:s([0-9A-Fa-f]{1,2}):l([0-9A-Fa-f]{1,2})(?::|$)`)
	reHEX    = regexp.MustCompile(`^HEX:([0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2})*)$`)
	rePUA    = regexp.MustCompile(`^PUA:([0-9A-Fa-f]+)`)
)

func CharToBytes(chr rune, charset string, version common.GameVersion) ([]uint, error) {
	if chr == '\n' {
		return []uint{0x03}, nil
	}

	indexValue, err := encoding.CharToByte(chr, charset, version)
	if err != nil {
		if errors.Is(err, encoding.ErrVersionNotFound) || errors.Is(err, encoding.ErrCharsetNotFound) {
			// erro de configuração: fatal
			return nil, fmt.Errorf("CharToBytes: %w", err)
		}
		// ErrCharNotFound: caractere ausente — dado ruim, o chamador decide
		// (pular, abortar, logar).
		return nil, fmt.Errorf("CharToBytes: %w", err)
	}

	return indexBytes(indexValue), nil
}

// indexBytes converte o índice flat do slot (código do jogo) nos bytes do
// encoding: single byte (0x30..0xFF), bytes de seção (0x2B/0xD0) ou extensão
// de 5 seções (0x04/0x410). É a inversa exata do decode em string_bytes.go.
func indexBytes(indexValue uint) []uint {
	if indexValue < 0x100 {
		return []uint{indexValue}
	}

	section := (indexValue - 0x30) / 0xD0
	byte1 := section + 0x2B
	byte2 := indexValue - (section * 0xD0)

	if byte1 <= 0x2F {
		return []uint{byte1, byte2}
	}

	adjustedValue := indexValue - 0x410

	if adjustedValue < 0x100 {
		return []uint{0x04, adjustedValue}
	}
	adjustedSection := (adjustedValue - 0x30) / 0xD0
	adjustedByte1 := adjustedSection + 0x2B
	adjustedByte2 := adjustedValue - (adjustedSection * 0xD0)
	return []uint{0x04, uint(adjustedByte1), uint(adjustedByte2)}
}

func GetChoicesInString(s string) int {
	choices := 0
	for {
		choiceTag := fmt.Sprintf("{CHOICE:%02X}", choices)
		if !strings.Contains(s, choiceTag) {
			break
		}
		choices++
	}
	return choices
}

func GetFirstChoiceInString(s string) (uint16, bool) {
	match := reChoice.FindStringSubmatch(s)
	if len(match) > 1 {
		if val, err := strconv.ParseUint(match[1], 16, 16); err == nil {
			return uint16(val), true
		}
	}
	return 0, false
}

func FillByteList(s string, buf *bytes.Buffer, charset string, version common.GameVersion) {
	version = common.CharsetVersion(version)
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		chr := runes[i]
		var cmdBytes []uint

		if chr == '{' {
			cmdBytes = ParseCommand(runes, i)
		}

		if cmdBytes == nil {
			charBytes, err := CharToBytes(chr, charset, version)
			if err != nil {
				if errors.Is(err, encoding.ErrVersionNotFound) || errors.Is(err, encoding.ErrCharsetNotFound) {
					common.LogError("FillByteList: configuração inválida: %v", err)
				} else {
					fmt.Fprintf(os.Stderr, "Unknown character %c at index %d in string %s\n", chr, i, s)
				}
			} else {
				for _, b := range charBytes {
					buf.WriteByte(byte(b))
				}
			}
		} else {
			for _, b := range cmdBytes {
				buf.WriteByte(byte(b))
			}
			i = getRunePosition(runes, '}', i)
		}
	}
	buf.WriteByte(0x00)
}

func StringToByteList(runes []rune, charset string, version common.GameVersion) ([]byte, error) {
	version = common.CharsetVersion(version)
	var buf bytes.Buffer
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		var cmdBytes []uint
		if r == '{' {
			cmdBytes = ParseCommand(runes, i)
		}
		if cmdBytes == nil {
			charBytes, err := CharToBytes(r, charset, version)
			if err != nil {
				if errors.Is(err, encoding.ErrVersionNotFound) || errors.Is(err, encoding.ErrCharsetNotFound) {
					return nil, fmt.Errorf("StringToByteList: configuração inválida: %w", err)
				}
				fmt.Fprintf(os.Stderr, "Unknown character %c at index %d in %q: %v\n", r, i, string(runes), err)
				continue
			}
			for _, b := range charBytes {
				buf.WriteByte(byte(b))
			}
		} else {
			for _, b := range cmdBytes {
				buf.WriteByte(byte(b))
			}
			i = getRunePosition(runes, '}', i)
		}
	}
	return buf.Bytes(), nil
}

func StringToBytes(s, charset string, version common.GameVersion) ([]byte, error) {
	runes := []rune(s)
	gameVersion := common.CharsetVersion(version)
	return StringToByteList(runes, charset, gameVersion)
}

// StringToStoredBytes devolve os bytes exatos que FillByteList gravaria para o
// texto (charset/versão aplicados e terminador 0x00 incluído): o texto vai
// direto a bytes — o chamador não cria buffer auxiliar, arquivo nem instância
// de domínio (FieldString, container etc.). Erro apenas em configuração
// (charset/versão); rune desconhecido é ignorado, igual a FillByteList.
func StringToStoredBytes(s, charset string, version common.GameVersion) ([]byte, error) {
	encoded, err := StringToBytes(s, charset, version)
	if err != nil {
		return nil, err
	}
	return append(encoded, 0x00), nil
}

// StringToByteSize devolve quantos bytes o texto ocupa armazenado — o mesmo
// que StringToStoredBytes, medido sem materializar a fatia para o chamador.
// Base para validar limites de binário (ex.: offsets uint16 pós-conversão de
// tags de controle) usando só o texto, sem rebuild nem objetos auxiliares.
func StringToByteSize(s, charset string, version common.GameVersion) (int, error) {
	encoded, err := StringToBytes(s, charset, version)
	if err != nil {
		return 0, err
	}
	return len(encoded) + 1, nil
}

func GetStringBytesAtLookupOffset(table []byte, offset int) []byte {
	if offset < 0 || offset >= len(table) {
		return nil
	}

	end := bytes.IndexByte(table[offset:], 0x00)
	if end == -1 {
		return table[offset:]
	}
	return table[offset : offset+end]
}

// macroBytes converte os campos hex (seção, linha) de um token MCR/MACRO nos
// bytes do jogo: seção 0x13.. e linha 0x30...
func macroBytes(secHex, lineHex string) (uint, uint, bool) {
	secVal, ok1 := parseHexByte(secHex)
	lineVal, ok2 := parseHexByte(lineHex)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return uint(secVal + 0x13), uint(lineVal + 0x30), true
}

// parseHexBytes converte os componentes hex de um token de bytes crus.
// "??" marca um byte ausente (dado truncado no binário): o token preserva os
// bytes que existem — "{UNKDBLCHR:2C:??}" re-codifica como [0x2C], e o
// re-decode do byte restante reemite o mesmo token (ponto fixo). Sem bytes a
// preservar ("{...:??}" puro), não há o que importar: devolve nil.
func parseHexBytes(parts []string) []uint {
	out := make([]uint, 0, len(parts))
	for _, part := range parts {
		if part == "??" {
			if len(out) == 0 {
				return nil
			}
			return out
		}
		val, ok := parseHexByte(part)
		if !ok {
			return nil
		}
		out = append(out, val)
	}
	return out
}

// parseHexByte converte UM componente hex de token (2 dígitos, 0..0xFF).
// Segundo retorno falso = token inválido (o chamador devolve nil e o
// ParseCommand ignora o token).
func parseHexByte(part string) (uint, bool) {
	val, err := strconv.ParseUint(part, 16, 8)
	if err != nil {
		return 0, false
	}
	return uint(val), true
}

func ParseCommand(runes []rune, startIndex int) []uint {
	if startIndex >= len(runes) {
		return nil
	}

	end := getRunePosition(runes, '}', startIndex)
	if end < 0 {
		return nil
	}

	cmd := string(runes[startIndex+1 : end])
	// Sem o guard de "??}": o nome no {PC:xx:??} é display (o encode usa o
	// índice), e os fallbacks ?? dos tokens de byte duplo re-codificam pelos
	// bytes do token. Tag desconhecida continua nil no fall-through do switch.

	switch {
	case cmd == "PAUSE":
		return []uint{0x01}
	case cmd == "BREAK":
		return []uint{0x02}
	case cmd == "\\n" || cmd == "TEXT_NEWLINE":
		return []uint{0x03}
	case cmd == "TEXT_ITALIC":
		return []uint{0x0E, 0x40}
	case cmd == "TEXT_NORMAL":
		return []uint{0x0E, 0x41}
	case cmd == "BLANK05":
		return []uint{0x05}
	case cmd == "BLANK0C":
		return []uint{0x0C}
	case cmd == "BLANK0F":
		return []uint{0x0F}
	case cmd == "BLANK11":
		return []uint{0x11}
	case strings.HasPrefix(cmd, "SPACE:"):
		val, ok := parseHexByte(cmd[6:])
		if !ok {
			return nil
		}
		return []uint{0x07, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "TIME:"):
		val, ok := parseHexByte(cmd[5:])
		if !ok {
			return nil
		}
		return []uint{0x09, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "CLR:"):
		clr := encoding.ColorToByte(cmd[4:])
		return []uint{0x0A, uint(clr)}
	case strings.HasPrefix(cmd, "COLOR:"):
		clr := encoding.ColorToByte(cmd[6:])
		return []uint{0x0A, uint(clr)}
	case strings.HasPrefix(cmd, "ICON:") || strings.HasPrefix(cmd, "BUTTON:"):
		parts := strings.SplitN(cmd, ":", 3)
		if len(parts) != 3 {
			fmt.Printf("Invalid ICON/BUTTON format: %s\n", cmd)
			return nil
		}
		iconIdx, ok := parseHexByte(parts[1])
		if !ok {
			fmt.Printf("Invalid hex value: %s\n", parts[1])
			return nil
		}
		return []uint{0x0B, uint(iconIdx)}
	case cmd == "CHOICE-END":
		return []uint{0x10, 0xFF}
	case strings.HasPrefix(cmd, "CHOICE:"):
		val, ok := parseHexByte(cmd[7:])
		if !ok {
			return nil
		}
		return []uint{0x10, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "VAR:"):
		val, ok := parseHexByte(cmd[4:])
		if !ok {
			return nil
		}
		return []uint{0x12, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "PC:"):
		parts := strings.SplitN(cmd, ":", 3)
		if len(parts) != 3 {
			fmt.Printf("Invalid PC format: %s\n", cmd)
			return nil
		}
		val, ok := parseHexByte(parts[1])
		if !ok {
			fmt.Printf("Invalid hex value: %s\n", parts[1])
			return nil
		}
		return []uint{0x13, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "MCR:"):
		matches := reMCR.FindStringSubmatch(cmd)
		if len(matches) != 3 {
			fmt.Printf("Invalid MCR format: %s\n", cmd)
			return nil
		}
		section, line, ok := macroBytes(matches[1], matches[2])
		if !ok {
			return nil
		}
		return []uint{section, line}
	case strings.HasPrefix(cmd, "MACRO:"):
		if len(cmd) < 12 {
			return nil
		}
		section, line, ok := macroBytes(cmd[7:9], cmd[10:12])
		if !ok {
			return nil
		}
		return []uint{section, line}
	case strings.HasPrefix(cmd, "KEY:"):
		val, ok := parseHexByte(cmd[4:6])
		if !ok {
			return nil
		}
		return []uint{0x23, uint(val + 0x30)}
	case strings.HasPrefix(cmd, "CMD:"):
		matches := reCmd.FindStringSubmatch(cmd)
		if len(matches) != 3 {
			fmt.Printf("Invalid CMD format: %s\n", cmd)
			return nil
		}
		cmdIdx, ok1 := parseHexByte(matches[1])
		argVal, ok2 := parseHexByte(matches[2])
		if !ok1 || !ok2 {
			return nil
		}
		return []uint{cmdIdx, uint(argVal + 0x30)}
	case strings.HasPrefix(cmd, "UNKOP:"):
		return parseHexBytes(strings.Split(cmd[6:], ":"))
	case strings.HasPrefix(cmd, "UNKTPLCHR:"):
		return parseHexBytes(strings.Split(cmd[10:], ":"))
	case strings.HasPrefix(cmd, "UNKDBLCHR:"):
		return parseHexBytes(strings.Split(cmd[10:], ":"))
	case strings.HasPrefix(cmd, "UNKCHR:"):
		return parseHexBytes(strings.Split(cmd[7:], ":"))
	case strings.HasPrefix(cmd, "HEX:"):
		matches := reHEX.FindStringSubmatch(cmd)
		if len(matches) != 2 {
			fmt.Printf("Invalid HEX format: %s\n", cmd)
			return nil
		}
		hexParts := strings.Split(matches[1], ":")
		result := make([]uint, len(hexParts))
		for i, part := range hexParts {
			if len(part) != 2 {
				fmt.Printf("Invalid HEX part: %s in command %s\n", part, cmd)
				return nil
			}
			val, _ := strconv.ParseUint(part, 16, 8)
			result[i] = uint(val)
		}
		return result
	case strings.HasPrefix(cmd, "PUA:"):
		// {PUA:CÓDIGO:CHAR} — slot duplicado da tabela de encoding.
		//
		// O decode (string_bytes.go, charOrPUAToken) emite este token quando o
		// byte lido decodifica a um rune cujo byte canônico do encode (1ª
		// ocorrência na tabela) é OUTRO byte: o slot é uma repetição. Emitir o
		// rune puro perderia o byte no re-import — e o byte exato importa
		// porque o font atlas pode desenhar glifos distintos por slot mesmo
		// quando a tabela os mapeia ao mesmo rune (ex.: Œ/œ onde a tabela diz
		// espaço; aspas duplas em 0x3C e 0x95 da tabela us).
		//
		// Semântica do re-import:
		//   - CÓDIGO é autoritativo: re-emite os bytes exatos do slot original
		//     (indexBytes), garantindo round-trip byte-a-byte.
		//   - CHAR é apenas informativo (o rune decodificado, para o texto
		//     extraído ficar pesquisável/legível) e é ignorado aqui.
		//   - Para trocar o caractere no texto, remova o token e escreva o
		//     caractere desejado — o encode vai ao slot canônico.
		//
		// O formato emitido pelo decode é sempre {PUA:CÓDIGO:CHAR}. O código é
		// hex de largura variável (%X — sempre ≥ 2 dígitos, pois os slots
		// começam em 0x30) e pode exceder 0xFF/0x410 em tabelas com seções,
		// daí indexBytes reconstruir os bytes 0x2B/0x04 quando preciso.
		matches := rePUA.FindStringSubmatch(cmd)
		if len(matches) != 2 {
			fmt.Printf("Invalid PUA format: %s\n", cmd)
			return nil
		}
		code, err := strconv.ParseUint(matches[1], 16, 32)
		if err != nil {
			fmt.Printf("Invalid PUA code: %s in command %s\n", matches[1], cmd)
			return nil
		}
		return indexBytes(uint(code))
	default:
		return nil
	}
}

func getRunePosition(runes []rune, target rune, start int) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

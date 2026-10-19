package converter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"ffxresources/backend/common"
	"ffxresources/backend/core/encoding"
	"ffxresources/backend/datastore"
	"fmt"
	"io"
	"strings"
)

func readOneByte(buf *bytes.Reader, out *byte) error {
	return binary.Read(buf, binary.LittleEndian, out)
}

// logDecodeError registra erro de configuração no decode (mapa/charset
// ausente). Caractere ausente é dado esperado (placeholder) e não loga.
func logDecodeError(err error) {
	if errors.Is(err, ffxencoding.ErrVersionNotFound) || errors.Is(err, ffxencoding.ErrCharsetNotFound) {
		common.LogError("decode: configuração inválida: %v", err)
	}
}

// charOrPUAToken devolve o texto do caractere do código do jogo: o próprio
// rune, ou o token {PUA:XX:CHAR} quando o slot é duplicado — o encode canônico
// usaria outro byte, então o token preserva o byte exato no round-trip
// (ver converter.ParseCommand, case "PUA:").
func charOrPUAToken(code uint, chr rune, charset string, version common.GameVersion) string {
	if canon, err := ffxencoding.CharToByte(chr, charset, version); err == nil && canon != code {
		return fmt.Sprintf("{PUA:%X:%c}", code, chr)
	}
	return string(chr)
}

func getStringAtLookupOffsetBinary(table []byte, offset int, localization string, version common.GameVersion) string {
	if offset < 0 || offset >= len(table) {
		return ""
	}

	var (
		out strings.Builder
		// O parâmetro é o charset na prática (field_string passa fs.Charset);
		// código de idioma (sp/fr/de/it) também é aceito. ResolveCharset
		// evita o bug de "default" cair na tabela us.
		charset           = ffxencoding.ResolveCharset(localization)
		gameVersion       = common.CharsetVersion(version)
		extraFiveSections bool
		buf               = bytes.NewReader(table[offset:])
	)

	for {
		var idx uint8
		if err := readOneByte(buf, &idx); err != nil || idx == 0x00 {
			break
		}

		var extraOffset uint = 0
		if extraFiveSections {
			extraOffset = 0x410
			extraFiveSections = false
		}

		switch {
		case idx >= 0x30:
			if chr, err := ffxencoding.ByteToChar(uint(idx)+extraOffset, charset, gameVersion); err == nil {
				out.WriteString(charOrPUAToken(uint(idx)+extraOffset, chr, charset, gameVersion))
			} else {
				logDecodeError(err)
				if extraOffset != 0 {
					out.WriteString(fmt.Sprintf("{UNKDBLCHR:04:%02X}", idx))
				} else {
					out.WriteString(fmt.Sprintf("{UNKCHR:%02X}", idx))
				}
			}

		// === NOVO: bytes duplos desconhecidos (0x06 e 0x26..0x2A) ===
		case idx == 0x06 || (idx >= 0x26 && idx < 0x2B):
			var lowByte uint8
			if err := readOneByte(buf, &lowByte); err != nil {
				if extraOffset != 0 {
					out.WriteString(fmt.Sprintf("{UNKTPLCHR:04:%02X:??}", idx))
				} else {
					out.WriteString(fmt.Sprintf("{UNKDBLCHR:%02X:??}", idx))
				}
				break
			}
			if extraOffset != 0 {
				out.WriteString(fmt.Sprintf("{UNKTPLCHR:04:%02X:%02X}", idx, lowByte))
			} else {
				out.WriteString(fmt.Sprintf("{UNKDBLCHR:%02X:%02X}", idx, lowByte))
			}

		// === Caracteres asiáticos de byte duplo (>= 0x2B) ===
		case idx >= 0x2B:
			var lowByte uint8
			if err := readOneByte(buf, &lowByte); err != nil {
				if extraOffset != 0 {
					out.WriteString(fmt.Sprintf("{UNKTPLCHR:04:%02X:??}", idx))
				} else {
					out.WriteString(fmt.Sprintf("{UNKDBLCHR:%02X:??}", idx))
				}
				break
			}
			section := uint(idx) - 0x2B
			actualIdx := section*0xD0 + uint(lowByte)
			newVar := actualIdx + extraOffset
			if chr, err := ffxencoding.ByteToChar(newVar, charset, gameVersion); err == nil {
				out.WriteString(charOrPUAToken(newVar, chr, charset, gameVersion))
			} else {
				logDecodeError(err)
				if extraOffset != 0 {
					out.WriteString(fmt.Sprintf("{UNKDBLCHR:04:%02X:%02X}", idx, lowByte))
				} else {
					out.WriteString(fmt.Sprintf("{UNKDBLCHR:%02X:%02X}", idx, lowByte))
				}
			}

		// === NOVO: quando extraOffset está ativo, bytes baixos viram caractere ===
		case extraOffset != 0:
			if chr, err := ffxencoding.ByteToChar(uint(idx)+extraOffset, charset, gameVersion); err == nil {
				out.WriteString(charOrPUAToken(uint(idx)+extraOffset, chr, charset, gameVersion))
			} else {
				logDecodeError(err)
				out.WriteString(fmt.Sprintf("{UNKDBLCHR:04:%02X}", idx))
			}

		case idx == 0x01:
			out.WriteString("{PAUSE}")
		case idx == 0x02:
			out.WriteString("{BREAK}")
		case idx == 0x03:
			if WriteLinebreaksAsCommands {
				out.WriteString("{TEXT_NEWLINE}")
			} else {
				out.WriteByte('\n')
			}
		case buf.Len() == 0:
			out.WriteString(fmt.Sprintf("{HEX:%02X}", idx))
		case idx == 0x04:
			extraFiveSections = true
		case idx == 0x05:
			out.WriteString("{BLANK05}")
		case idx == 0x07:
			var pixels uint8
			if err := readOneByte(buf, &pixels); err != nil {
				out.WriteString("{SPACE:??}")
				break
			}
			out.WriteString(fmt.Sprintf("{SPACE:%02X}", pixels-0x30))
		case idx == 0x09:
			var varIdx uint8
			if err := readOneByte(buf, &varIdx); err != nil {
				out.WriteString("{TIME:??}")
				break
			}
			out.WriteString(fmt.Sprintf("{TIME:%02X}", varIdx-0x30))
		case idx == 0x0A:
			var clr uint8
			if err := readOneByte(buf, &clr); err != nil {
				out.WriteString("{CLR:??}")
				break
			}
			out.WriteString(ffxencoding.GetColorString(clr))
		case idx == 0x0B:
			var icon uint8
			if err := readOneByte(buf, &icon); err != nil {
				out.WriteString("{ICON:??}")
				break
			}
			if icon < 0x80 {
				out.WriteString(fmt.Sprintf("{BUTTON:%02X:%s}", icon, ffxencoding.GetButtonName(icon)))
			} else {
				out.WriteString(fmt.Sprintf("{ICON:%02X:%s}", icon, ffxencoding.GetIconName(icon)))
			}
		case idx == 0x0C:
			out.WriteString("{BLANK0C}")
		case idx == 0x0F:
			out.WriteString("{BLANK0F}")
		case idx == 0x0E:
			var style uint8
			if err := readOneByte(buf, &style); err != nil {
				out.WriteString("{CMD:0E:??}")
				break
			}
			switch style {
			case 0x40:
				out.WriteString("{TEXT_ITALIC}")
			case 0x41:
				out.WriteString("{TEXT_NORMAL}")
			default:
				out.WriteString(fmt.Sprintf("{CMD:0E:%02X}", style-0x30))
			}
		case idx == 0x10:
			var rawValue uint8
			if err := readOneByte(buf, &rawValue); err != nil {
				if err == io.EOF {
					out.WriteString(fmt.Sprintf("{HEX:%02X}", idx))
				} else {
					out.WriteString("{CHOICE:??}")
				}
				break
			}
			if rawValue == 0xFF {
				out.WriteString("{CHOICE-END}")
				break
			}
			choiceIdx := rawValue - 0x30
			out.WriteString(fmt.Sprintf("{CHOICE:%02X}", choiceIdx))
		case idx == 0x11:
			out.WriteString("{BLANK11}")
		case idx == 0x12:
			var varIdx uint8
			if err := readOneByte(buf, &varIdx); err != nil {
				out.WriteString("{VAR:??}")
				break
			}
			out.WriteString(fmt.Sprintf("{VAR:%02X}", varIdx-0x30))
		case idx == 0x13 && buf.Len() > 0:
			var rawValue uint8
			if err := readOneByte(buf, &rawValue); err != nil {
				out.WriteString("{HEX:13}")
				break
			}
			// Faixa simétrica ao encode (PC:xx → 0x13, xx+0x30): 0x30..0x46
			// cobre os speakers das duas versões (FFX-2 chega a DUMMY10). O
			// nome é só display — speaker fora do mapa da versão vira "?" e
			// o round-trip continua byte-exato pelo índice.
			if rawValue >= 0x30 && rawValue <= 0x46 {
				pcIdx := rawValue - 0x30
				out.WriteString(fmt.Sprintf("{PC:%02X:%s}", pcIdx, ffxencoding.GetPlayerChar(pcIdx, gameVersion)))
				break
			}
			// Fora da faixa: token genérico de byte duplo, que re-codifica
			// os bytes exatos (ver ParseCommand UNKDBLCHR).
			out.WriteString(fmt.Sprintf("{UNKDBLCHR:13:%02X}", rawValue))
		case idx >= 0x13 && idx <= 0x22:
			var section uint = uint(idx) - 0x13
			var line byte
			if err := readOneByte(buf, &line); err != nil {
				fmt.Println("Error reading line number for MCR command:", err)
				out.WriteString(fmt.Sprintf("{HEX:%02X}", idx))
				break
			}
			lineAdjusted := line - 0x30
			out.WriteString(fmt.Sprintf("{MCR:s%02X:l%02X", section, lineAdjusted))
			if datastore.HasMacros(gameVersion) {
				out.WriteString(":")
				index := int(section*0x100 + uint(lineAdjusted))
				if macro, exist := datastore.GetMacro(gameVersion, index); exist && macro != nil {
					if macroContent, exists := macro.GetLocalizedContent(localization); exists && macroContent != nil {
						out.WriteString(`"`)
						localizedContent, _ := macro.GetLocalizedContent(localization)
						out.WriteString(localizedContent.String())
						out.WriteString(`"`)
					} else {
						out.WriteString("<Missing>")
					}
				} else {
					out.WriteString("<Missing>")
				}
			}
			out.WriteString("}")
		case idx == 0x23:
			var varIdx uint8
			if err := readOneByte(buf, &varIdx); err != nil {
				out.WriteString("{KEY:??}")
				break
			}
			varIdx -= 0x30
			out.WriteString(fmt.Sprintf("{KEY:%02X", varIdx))
			if keyItem := datastore.KeyItems.Get(int(varIdx)); keyItem != nil {
				out.WriteString(fmt.Sprintf(`:"%s"`, keyItem.GetName(localization)))
			}
			out.WriteString("}")
		default:
			var nextByte byte
			if err := readOneByte(buf, &nextByte); err != nil {
				out.WriteString(fmt.Sprintf("{CMD:%02X:??}", idx))
				break
			}
			out.WriteString(fmt.Sprintf("{CMD:%02X:%02X}", idx, nextByte-0x30))
		}
	}

	return out.String()
}

func BytesToString(rawData []byte, localization string, version common.GameVersion) string {
	return getStringAtLookupOffsetBinary(rawData, 0, localization, common.CharsetVersion(version))
}

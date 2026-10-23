package converter

import (
	"errors"
	"ffxresources/backend/common"
	"ffxresources/backend/core/encoding"
	"ffxresources/backend/datastore"
	"fmt"
	"strings"
)

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

// binaryDecoder é o estado de uma extração: o cursor na tabela, o builder de
// saída e o contexto de charset/versão/localização. A flag fiveSections é
// ligada pelo byte 0x04 e consumida pelo próximo byte lido.
type binaryDecoder struct {
	buf          []byte
	pos          int
	out          strings.Builder
	charset      string
	gameVersion  common.GameVersion
	localization string
	fiveSections bool
}

func getStringAtLookupOffsetBinary(table []byte, offset int, localization string, version common.GameVersion) string {
	if offset < 0 || offset >= len(table) {
		return ""
	}

	d := &binaryDecoder{
		buf: table[offset:],
		// O parâmetro é o charset na prática (field_string passa fs.Charset);
		// código de idioma (sp/fr/de/it) também é aceito. ResolveCharset
		// evita o bug de "default" cair na tabela us.
		charset:      ffxencoding.ResolveCharset(localization),
		gameVersion:  common.CharsetVersion(version),
		localization: localization,
	}

	for {
		idx, ok := d.next()
		if !ok {
			break
		}
		d.decode(idx)
	}

	return d.out.String()
}

// next lê o próximo byte da tabela. ok=false no fim ou no terminador 0x00.
func (d *binaryDecoder) next() (byte, bool) {
	idx, ok := d.read()
	if !ok || idx == 0x00 {
		return 0, false
	}
	return idx, true
}

// read lê um byte cru. ok=false no fim da tabela — usado pelos casos que vêm
// ANTES da sentinela de fim (chars/bytes duplos), onde o truncamento tem token
// próprio (??).
func (d *binaryDecoder) read() (byte, bool) {
	if d.pos >= len(d.buf) {
		return 0, false
	}
	b := d.buf[d.pos]
	d.pos++
	return b, true
}

// remaining devolve os bytes ainda não lidos (sentinela de fim).
func (d *binaryDecoder) remaining() int {
	return len(d.buf) - d.pos
}

// arg lê o argumento de um comando. Só é chamado depois da sentinela de fim
// (decodeCommand), então o buffer sempre tem o byte; truncar aqui é
// inalcançável e o byte devolvido é 0.
func (d *binaryDecoder) arg() byte {
	b, _ := d.read()
	return b
}

// takeFiveSectionsOffset consome a flag ligada por 0x04: o próximo byte é
// deslocado em 0x410 (extensão de 5 seções). A flag vale para UM byte.
func (d *binaryDecoder) takeFiveSectionsOffset() uint {
	if !d.fiveSections {
		return 0
	}
	d.fiveSections = false
	return 0x410
}

// decode é o dispatcher. A ORDEM dos guards é o contrato do formato:
//
//  1. byte de texto (>= 0x30);
//  2. prefixo 0x04 (2 ou 3 bytes: extensão de 5 seções);
//  3. seção 0x2C..0x2F (2 bytes);
//  4. ops de 2 bytes (0x06, 0x08, 0x0C..0x0F, 0x11, 0x24..0x2B);
//  5. comandos sem argumento que valem no fim da tabela (0x01..0x03);
//  6. sentinela de fim (último byte sem terminador);
//  7. comandos.
//
// 0x2B NÃO é seção sem o prefixo 0x04: é op (Fahrenheit DecodingGamecode*).
// Trocar essa ordem muda o resultado.
func (d *binaryDecoder) decode(idx byte) {
	extraOffset := d.takeFiveSectionsOffset()

	switch {
	case idx >= 0x30:
		d.decodeTextByte(idx, extraOffset)
	case extraOffset != 0:
		// Prefixo 0x04: 0x2B..0x2F continuam seção (3 bytes); o resto é
		// extensão de um byte só (0x04 + slot).
		if idx >= 0x2B {
			d.decodeSectionByte(idx, extraOffset)
		} else {
			d.decodeExtendedByte(idx, extraOffset)
		}
	case idx >= 0x2C && idx <= 0x2F:
		d.decodeSectionByte(idx, 0)
	case idx == 0x01:
		d.out.WriteString("{PAUSE}")
	case idx == 0x02:
		d.out.WriteString("{BREAK}")
	case idx == 0x03:
		if WriteLinebreaksAsCommands {
			d.out.WriteString("{TEXT_NEWLINE}")
		} else {
			d.out.WriteByte('\n')
		}
	case d.remaining() == 0:
		d.writeHex(idx)
	case idx == 0x06 || idx == 0x08 ||
		(idx >= 0x0C && idx <= 0x0F) || idx == 0x11 ||
		(idx >= 0x24 && idx <= 0x2B):
		d.decodeOp(idx)
	default:
		d.decodeCommand(idx)
	}
}

// tryWriteChar resolve o slot e escreve o rune (ou o token PUA). Falha só
// marca o erro de configuração; o placeholder é responsabilidade do chamador,
// que conhece o token certo para o contexto.
func (d *binaryDecoder) tryWriteChar(code uint) bool {
	chr, err := ffxencoding.ByteToChar(code, d.charset, d.gameVersion)
	if err != nil {
		logDecodeError(err)
		return false
	}
	d.out.WriteString(charOrPUAToken(code, chr, d.charset, d.gameVersion))
	return true
}

// decodeTextByte: byte de texto (0x30..0xFF), com ou sem o deslocamento de 5
// seções.
func (d *binaryDecoder) decodeTextByte(idx byte, extraOffset uint) {
	code := uint(idx) + extraOffset
	if d.tryWriteChar(code) {
		return
	}
	if extraOffset != 0 {
		fmt.Fprintf(&d.out, "{UNKDBLCHR:04:%02X}", idx)
		return
	}
	fmt.Fprintf(&d.out, "{UNKCHR:%02X}", idx)
}

// decodeOp lê um op de 2 bytes (código + argumento cru). A referência é o
// decodificador da Fahrenheit (DecodingGamecode*): 0x06, 0x08, 0x0C..0x0F,
// 0x11 e 0x24..0x2B são operadores de 1 argumento — NÃO caracteres de 2
// bytes. Confirmado nos binários: em azit0300 (JP) o texto "？０！" é
// 13 34 03 2D 9E 2A 30 3B 00: 0x2A 0x30 é o op 0x2A com arg 0x30, não um
// caractere CJK. O token preserva os dois bytes crus no round-trip.
func (d *binaryDecoder) decodeOp(idx byte) {
	arg, ok := d.read()
	if !ok {
		fmt.Fprintf(&d.out, "{UNKOP:%02X:??}", idx)
		return
	}
	if idx == 0x0E {
		d.writeStyle(arg)
		return
	}
	fmt.Fprintf(&d.out, "{UNKOP:%02X:%02X}", idx, arg)
}

// decodeSectionByte: caractere de seção (0x2C..0x2F, e 0x2B só com o
// prefixo 0x04). O índice flat é (seção * 0xD0) + byte baixo.
func (d *binaryDecoder) decodeSectionByte(idx byte, extraOffset uint) {
	lowByte, ok := d.read()
	if !ok {
		if extraOffset != 0 {
			fmt.Fprintf(&d.out, "{UNKTPLCHR:04:%02X:??}", idx)
		} else {
			fmt.Fprintf(&d.out, "{UNKDBLCHR:%02X:??}", idx)
		}
		return
	}

	section := uint(idx) - 0x2B
	code := section*0xD0 + uint(lowByte) + extraOffset
	if d.tryWriteChar(code) {
		return
	}
	if extraOffset != 0 {
		fmt.Fprintf(&d.out, "{UNKDBLCHR:04:%02X:%02X}", idx, lowByte)
	} else {
		fmt.Fprintf(&d.out, "{UNKDBLCHR:%02X:%02X}", idx, lowByte)
	}
}

// decodeExtendedByte: byte baixo com a flag de 5 seções ativa (0x04 anterior).
func (d *binaryDecoder) decodeExtendedByte(idx byte, extraOffset uint) {
	if d.tryWriteChar(uint(idx) + extraOffset) {
		return
	}
	fmt.Fprintf(&d.out, "{UNKDBLCHR:04:%02X}", idx)
}

// decodeCommand trata os comandos de controle. Só é chamado depois da
// sentinela de fim, então há pelo menos um byte para os argumentos.
func (d *binaryDecoder) decodeCommand(idx byte) {
	if idx >= 0x14 && idx <= 0x22 {
		d.writeMacro(idx, d.arg())
		return
	}

	switch idx {
	case 0x04:
		d.fiveSections = true
	case 0x05:
		d.out.WriteString("{BLANK05}")
	case 0x07:
		fmt.Fprintf(&d.out, "{SPACE:%02X}", d.arg()-0x30)
	case 0x09:
		fmt.Fprintf(&d.out, "{TIME:%02X}", d.arg()-0x30)
	case 0x0A:
		d.out.WriteString(ffxencoding.GetColorString(d.arg()))
	case 0x0B:
		d.writeIconOrButton(d.arg())
	case 0x10:
		d.writeChoice(d.arg())
	case 0x12:
		fmt.Fprintf(&d.out, "{VAR:%02X}", d.arg()-0x30)
	case 0x13:
		d.writeSpeaker(d.arg())
	case 0x23:
		d.writeKeyItem(d.arg())
	default:
		fmt.Fprintf(&d.out, "{CMD:%02X:%02X}", idx, d.arg()-0x30)
	}
}

// writeIconOrButton: abaixo de 0x80 é botão, a partir disso é ícone.
func (d *binaryDecoder) writeIconOrButton(icon byte) {
	if icon < 0x80 {
		fmt.Fprintf(&d.out, "{BUTTON:%02X:%s}", icon, ffxencoding.GetButtonName(icon))
		return
	}
	fmt.Fprintf(&d.out, "{ICON:%02X:%s}", icon, ffxencoding.GetIconName(icon))
}

// writeStyle: 0x40 itálico, 0x41 normal, resto genérico.
func (d *binaryDecoder) writeStyle(style byte) {
	switch style {
	case 0x40:
		d.out.WriteString("{TEXT_ITALIC}")
	case 0x41:
		d.out.WriteString("{TEXT_NORMAL}")
	default:
		fmt.Fprintf(&d.out, "{CMD:0E:%02X}", style-0x30)
	}
}

// writeChoice: 0xFF encerra a lista; os demais são índices.
func (d *binaryDecoder) writeChoice(rawValue byte) {
	if rawValue == 0xFF {
		d.out.WriteString("{CHOICE-END}")
		return
	}
	fmt.Fprintf(&d.out, "{CHOICE:%02X}", rawValue-0x30)
}

// writeSpeaker: 0x13 + personagem. A faixa 0x30..0x46 cobre os speakers das
// duas versões (FFX-2 chega a DUMMY10); o nome é só display — speaker fora do
// mapa da versão vira "?" e o round-trip continua byte-exato pelo índice.
func (d *binaryDecoder) writeSpeaker(rawValue byte) {
	if rawValue >= 0x30 && rawValue <= 0x46 {
		pcIdx := rawValue - 0x30
		fmt.Fprintf(&d.out, "{PC:%02X:%s}", pcIdx, ffxencoding.GetPlayerChar(pcIdx, d.gameVersion))
		return
	}
	// Fora da faixa: token genérico de byte duplo, que re-codifica os bytes
	// exatos (ver ParseCommand UNKDBLCHR).
	fmt.Fprintf(&d.out, "{UNKDBLCHR:13:%02X}", rawValue)
}

// writeMacro: 0x13..0x22 carregam seção/linha da macro. Quando o dicionário da
// versão está carregado, o texto localizado entra entre aspas; macro ausente
// (ou idioma sem conteúdo) vira <Missing>.
func (d *binaryDecoder) writeMacro(idx byte, line byte) {
	section := uint(idx) - 0x13
	lineAdjusted := line - 0x30

	fmt.Fprintf(&d.out, "{MCR:s%02X:l%02X", section, lineAdjusted)
	if datastore.HasMacros(d.gameVersion) {
		d.out.WriteByte(':')
		if content, ok := d.macroContent(section, lineAdjusted); ok && content != nil {
			d.out.WriteByte('"')
			d.out.WriteString(content.String())
			d.out.WriteByte('"')
		} else {
			d.out.WriteString("<Missing>")
		}
	}
	d.out.WriteByte('}')
}

// macroContent resolve a macro pelo índice flat (seção*0x100 + linha) e o
// conteúdo no idioma da extração.
func (d *binaryDecoder) macroContent(section uint, line byte) (datastore.IGlobalMacroString, bool) {
	index := int(section*0x100 + uint(line))
	macro, exists := datastore.GetMacro(d.gameVersion, index)
	if !exists || macro == nil {
		return nil, false
	}
	return macro.GetLocalizedContent(d.localization)
}

// writeKeyItem: 0x23 + índice. O nome só entra quando o catálogo de key items
// está carregado.
func (d *binaryDecoder) writeKeyItem(rawValue byte) {
	keyIdx := rawValue - 0x30
	fmt.Fprintf(&d.out, "{KEY:%02X", keyIdx)
	if keyItem := datastore.KeyItems.Get(int(keyIdx)); keyItem != nil {
		fmt.Fprintf(&d.out, `:"%s"`, keyItem.GetName(d.localization))
	}
	d.out.WriteByte('}')
}

// writeHex é a sentinela do último byte sem terminador: qualquer byte que
// chegue aqui sai como {HEX:xx}, sem consumir argumento.
func (d *binaryDecoder) writeHex(idx byte) {
	fmt.Fprintf(&d.out, "{HEX:%02X}", idx)
}

func BytesToString(rawData []byte, localization string, version common.GameVersion) string {
	return getStringAtLookupOffsetBinary(rawData, 0, localization, common.CharsetVersion(version))
}

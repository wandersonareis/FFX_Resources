// Package ddsphyre lê, extrai e (re)empacota texturas PhyreEngine do
// FDX HD no formato DX11 (*.dds.phyre).
//
// O container é auto-descritivo: o bloco *namespace* declara classes,
// membros e a tabela de strings, de onde saem os offsets de
// width/height/mips — nada é hardcoded. O payload a partir de dataOffset
// JÁ É um arquivo DDS, então extrair é header DDS + cópia bruta e
// reempacotar é reescrever o payload e os campos que ele altera.
//
// Port fiel de dds-phyre-tool (PhyrePlatform.cpp / PhyrePlatformDX11.cpp):
//   - magic little-endian 0x50485952 (no disco os bytes são "RYHP";
//     arquivo big-endian "PHYR" é rejeitado);
//   - platformId 0x44583131 ("DX11" no disco);
//   - size == 84 (21 campos uint32) e namespaceSize cabendo no arquivo.
package ddsphyre

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Constantes do container DX11 (todas little-endian).
const (
	// PhyreMagic é o magic lido little-endian: 0x50485952 = "PHYR" em BE.
	PhyreMagic = 0x50485952
	// phyreMagicBE é o mesmo magic gravado big-endian (não suportado).
	phyreMagicBE = 0x52594850
	// PlatformIDDX11 é o platformId do DX11: 0x44583131 = "DX11" em BE.
	PlatformIDDX11 = 0x44583131

	// Suffix é a extensão dos arquivos de textura.
	Suffix = ".dds.phyre"

	dx11HeaderSize   = 84
	nsHeaderSize     = 32
	classDescLen     = 36
	memberDescLen    = 24
	instanceDescLen  = 36
	userFixupLen     = 12
	ddsHeaderLen     = 128
	ddsHeaderBodyLen = 124
)

// Offsets dos campos do _tDX11Header (21 × uint32).
const (
	offMagic               = 0
	offSize                = 4
	offNamespaceSize       = 8
	offPlatformID          = 12
	offInstanceListCount   = 16
	offArrayFixupSize      = 20
	offPointerFixupSize    = 28
	offPointerArrayFixupSz = 36
	offUserFixupCount      = 48
	offUserFixupDataSize   = 52
	offTotalDataSize       = 56
	offMaxTextureBufSize   = 80
)

// Offsets dos campos do _tNamespaceHeader (8 × uint32).
const (
	nsOffSize               = 4
	nsOffTypeCount          = 8
	nsOffClassCount         = 12
	nsOffClassDataMemberCnt = 16
	nsOffStringTableSize    = 20
	nsOffDefaultBufferCount = 24
	nsOffDefaultBufferSize  = 28
)

// Offsets dentro de um _tNamespaceClassDescriptor e de um
// _tNamespaceDataMember.
const (
	classOffNameOffset      = 8
	classOffDataMemberCount = 12
	memberOffNameOffset     = 0
	memberOffValueOffset    = 8
	instanceOffClassID      = 0
	instanceOffSize         = 8
	userFixupOffTypeID      = 0
	userFixupOffSize        = 4
	userFixupOffOffset      = 8
)

// Texture é o resultado do parse de um .dds.phyre.
type Texture struct {
	// Format é a string do Phyre ("DXT1", "DXT3", "DXT5", "BC5",
	// "ARGB8", "A8", "L8"), vinda do fixup[1] → PTextureFormatBase.
	Format           string
	Width            uint32
	Height           uint32
	MipmapCount      uint32
	MaxMipmapLevel   uint32
	TextureFlags     uint32
	MaxTextureBuffer uint32

	// Offsets absolutos (byte 0 do arquivo).
	InfoOffset      uint64
	DataOffset      uint64
	FixupOffset     uint64
	FixupDataOffset uint64

	// Offsets dos membros dentro do objeto, relativos a InfoOffset.
	widthOffset        uint64
	heightOffset       uint64
	mipmapCountOffset  uint64
	maxMipmapLevelOffs uint64

	// Raw é o arquivo completo; Payload é Raw[DataOffset:] — o DDS puro.
	Raw     []byte
	Payload []byte

	// AssetPath é o caminho interno do asset embutido na string table
	// (ex.: "PS3Data/help_es/helpgra/help10.dds"). Só INFO de exibição:
	// o prefixo "PS3Data/" é o caminho que o jogo lê dentro da árvore e
	// nunca participa do encode (Import/Save trabalham pelo id da árvore).
	// "" quando o container não declara nenhum.
	AssetPath string
}

// le32 lê um uint32 little-endian com checagem de faixa.
func le32(b []byte, off uint64) (uint32, error) {
	if off > uint64(len(b)) || uint64(len(b))-off < 4 {
		return 0, fmt.Errorf("offset %d fora do buffer (%d bytes)", off, len(b))
	}
	return binary.LittleEndian.Uint32(b[int(off):]), nil
}

func mustLE32(b []byte, off uint64) uint32 {
	v, _ := le32(b, off)
	return v
}

// put32 grava um uint32 little-endian sem checagem (só em buffers que o
// próprio pacote montou com tamanho conhecido).
func put32(b []byte, off uint64, v uint32) {
	binary.LittleEndian.PutUint32(b[int(off):], v)
}

// cstr extrai uma string terminada em NUL a partir de off. ok=false
// significa "fora do buffer" (offset inválido no namespace).
func cstr(b []byte, off uint64) (string, bool) {
	if off >= uint64(len(b)) {
		return "", false
	}
	rest := b[int(off):]
	for i := range rest {
		if rest[i] == 0 {
			return string(rest[:i]), true
		}
	}
	return string(rest), true
}

// assetPathOf devolve o caminho interno do asset gravado no container
// (ex.: "PS3Data/help_es/helpgra/help10.dds") — é o caminho que o próprio
// jogo usa na leitura, independente de onde o arquivo está agora: serve
// inclusive para um .dds.phyre avulso, aberto sem árvore de diretórios.
//
// A string mora DEPOIS da string table, no bloco de instância/dados, então
// a varredura é pelo container inteiro até o payload — nunca pelos pixels,
// que vêm depois de DataOffset. Um container declara UMA string ".dds"
// (verificado em toda a árvore das duas versões); havendo mais de uma,
// prefere a que tem pasta (caminho completo) e cai no basename. Só
// ASCII imprimível conta: lixo binário que termine em ".dds" por acaso é
// descartado. Container que traz apenas o basename (caso raro, ex.:
// "AutoSaving.dds") volta igual — sem pasta não há idioma a tirar dali e
// o chamador recorre ao id da árvore. "" = sem candidato.
func assetPathOf(container []byte) string {
	only := ""
	start := 0
	for i := 0; i <= len(container); i++ {
		if i < len(container) && container[i] != 0 {
			continue
		}
		chunk := container[start:i]
		start = i + 1

		if !isPrintASCII(chunk) || len(chunk) < len(".dds") ||
			!strings.EqualFold(string(chunk[len(chunk)-len(".dds"):]), ".dds") {
			continue
		}
		s := string(chunk)
		if strings.ContainsRune(s, '/') {
			return s
		}
		if only == "" {
			only = s
		}
	}
	return only
}

// isPrintASCII diz se a região é toda ASCII imprimível (0x20–0x7E) — o que
// separa as strings de verdade do lixo binário que ocupa os mesmos bytes.
func isPrintASCII(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// IsPhyre é a checagem rápida (equivalente a isFormatSupported): magic,
// platformId, size e namespace cabendo no arquivo.
func IsPhyre(data []byte) bool {
	if len(data) < dx11HeaderSize {
		return false
	}
	ns := uint64(mustLE32(data, offNamespaceSize))
	return mustLE32(data, offMagic) == PhyreMagic &&
		mustLE32(data, offSize) == dx11HeaderSize &&
		mustLE32(data, offPlatformID) == PlatformIDDX11 &&
		uint64(dx11HeaderSize)+ns <= uint64(len(data))
}

// ParseFile lê um .dds.phyre do disco.
func ParseFile(path string) (*Texture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", filepath.Base(path), err)
	}
	return Parse(raw)
}

// Parse decodifica o container DX11 e devolve os metadados da textura mais
// o payload DDS (Raw[DataOffset:]).
func Parse(data []byte) (*Texture, error) {
	if len(data) < dx11HeaderSize {
		return nil, fmt.Errorf("dds.phyre: arquivo de %d bytes não cabe no header DX11 (%d)", len(data), dx11HeaderSize)
	}
	if magic := mustLE32(data, offMagic); magic != PhyreMagic {
		if magic == phyreMagicBE {
			return nil, fmt.Errorf("dds.phyre: magic big-endian (PHYR) não é suportado")
		}
		return nil, fmt.Errorf("dds.phyre: magic inválido 0x%08X (esperado 0x%08X)", magic, PhyreMagic)
	}
	if size := mustLE32(data, offSize); size != dx11HeaderSize {
		return nil, fmt.Errorf("dds.phyre: size=%d (esperado %d)", size, dx11HeaderSize)
	}
	if platform := mustLE32(data, offPlatformID); platform != PlatformIDDX11 {
		return nil, fmt.Errorf("dds.phyre: platformId=0x%08X (só DX11=0x%08X é suportado)", platform, PlatformIDDX11)
	}

	nsSize := uint64(mustLE32(data, offNamespaceSize))
	if uint64(dx11HeaderSize)+nsSize > uint64(len(data)) {
		return nil, fmt.Errorf("dds.phyre: namespace de %d bytes não cabe no arquivo de %d", nsSize, len(data))
	}

	ns, err := newNamespace(data[dx11HeaderSize:int(uint64(dx11HeaderSize)+nsSize)])
	if err != nil {
		return nil, err
	}

	instCount := uint64(mustLE32(data, offInstanceListCount))
	userFixupCount := uint64(mustLE32(data, offUserFixupCount))
	userFixupDataSize := uint64(mustLE32(data, offUserFixupDataSize))
	totalDataSize := uint64(mustLE32(data, offTotalDataSize))

	// A lista de instâncias começa logo após o namespace; a instância
	// PTexture2D é a primeira cujo nome de classe casa (1-based).
	instBase := uint64(dx11HeaderSize) + nsSize
	var textureInstanceStart uint64
	found := false
	for i := uint64(0); i < instCount; i++ {
		off := instBase + i*instanceDescLen
		classID, err := le32(data, off+instanceOffClassID)
		if err != nil {
			return nil, fmt.Errorf("dds.phyre: instância %d: %w", i, err)
		}
		if classID == 0 || uint64(classID) > ns.classCount {
			return nil, fmt.Errorf("dds.phyre: instância %d tem classId %d inválido", i, classID)
		}
		name, err := ns.className(classID)
		if err != nil {
			return nil, err
		}
		if name == "PTexture2D" {
			found = true
			break
		}
		sizeField, err := le32(data, off+instanceOffSize)
		if err != nil {
			return nil, fmt.Errorf("dds.phyre: instância %d: %w", i, err)
		}
		textureInstanceStart += uint64(sizeField)
	}
	if !found {
		return nil, fmt.Errorf("dds.phyre: instância PTexture2D não encontrada (arquivo não suportado)")
	}

	infoOffset := instBase + instCount*instanceDescLen + textureInstanceStart
	members, err := ns.textureMembers()
	if err != nil {
		return nil, err
	}
	t := &Texture{
		InfoOffset:         infoOffset,
		widthOffset:        members.width,
		heightOffset:       members.height,
		mipmapCountOffset:  members.mipmapCount,
		maxMipmapLevelOffs: members.maxMipmapLevel,
		Raw:                data,
	}
	if t.Width, err = le32(data, infoOffset+members.width); err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo width: %w", err)
	}
	if t.Height, err = le32(data, infoOffset+members.height); err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo height: %w", err)
	}
	if t.MipmapCount, err = le32(data, infoOffset+members.mipmapCount); err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo mipmapCount: %w", err)
	}
	if t.MaxMipmapLevel, err = le32(data, infoOffset+members.maxMipmapLevel); err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo maxMipLevel: %w", err)
	}
	if t.TextureFlags, err = le32(data, infoOffset+members.textureFlags); err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo textureFlags: %w", err)
	}
	t.MaxTextureBuffer = mustLE32(data, offMaxTextureBufSize)

	// User fixup: os dados vêm antes das entradas; o fixup[1] aponta para
	// a string de formato (fixup[0] é o nome da classe).
	t.FixupDataOffset = instBase + instCount*instanceDescLen + totalDataSize
	if t.FixupDataOffset+userFixupDataSize > uint64(len(data)) {
		return nil, fmt.Errorf("dds.phyre: user fixup data de %d bytes não cabe no arquivo", userFixupDataSize)
	}
	t.FixupOffset = t.FixupDataOffset + userFixupDataSize
	if userFixupCount < 2 {
		return nil, fmt.Errorf("dds.phyre: %d user fixups (esperado ao menos 2)", userFixupCount)
	}
	fixup, err := le32(data, t.FixupOffset+userFixupLen+userFixupOffTypeID)
	if err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo fixup[1]: %w", err)
	}
	if uint64(fixup) >= ns.typeCount {
		return nil, fmt.Errorf("dds.phyre: fixup[1].typeId %d >= typeCount %d", fixup, ns.typeCount)
	}
	typeName, err := ns.typeName(fixup)
	if err != nil {
		return nil, err
	}
	if typeName != "PTextureFormatBase" {
		return nil, fmt.Errorf("dds.phyre: formato de textura não encontrado (typeId %d = %q)", fixup, typeName)
	}
	fixupSize, err := le32(data, t.FixupOffset+userFixupLen+userFixupOffSize)
	if err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo fixup[1].size: %w", err)
	}
	fixupOff, err := le32(data, t.FixupOffset+userFixupLen+userFixupOffOffset)
	if err != nil {
		return nil, fmt.Errorf("dds.phyre: lendo fixup[1].offset: %w", err)
	}
	fixupData := data[t.FixupDataOffset:int(t.FixupDataOffset+userFixupDataSize)]
	if uint64(fixupOff) >= uint64(len(fixupData)) {
		return nil, fmt.Errorf("dds.phyre: fixup[1].offset %d fora do buffer de %d bytes", fixupOff, len(fixupData))
	}
	format, ok := cstr(fixupData, uint64(fixupOff))
	if !ok || format == "" {
		return nil, fmt.Errorf("dds.phyre: string de formato vazia")
	}
	_ = fixupSize
	t.Format = format

	// dataOffset: soma de todas as seções até o payload.
	dataOffset, err := dataOffsetOf(data)
	if err != nil {
		return nil, fmt.Errorf("dds.phyre: %w", err)
	}
	t.DataOffset = dataOffset
	t.Payload = data[int(dataOffset):]
	// Caminho interno do asset (info de exibição): mora no bloco do
	// container, antes dos pixels — por isso a varredura é até o payload.
	t.AssetPath = assetPathOf(data[:int(dataOffset)])

	return t, nil
}

// dataOffsetOf devolve o offset do payload — o ponto onde acaba o container
// e começa a imagem — lendo SOMENTE o header DX11, sem decodificar o
// namespace.
//
// É a mesma fórmula do Parse, isolada porque o índice de duplicatas varre
// milhares de arquivos e só precisa desse limite: o payload é a imagem, o
// namespace é o que carrega o nome/caminho do arquivo (que muda de cópia
// para cópia e por isso torna inútil o hash do arquivo inteiro).
func dataOffsetOf(data []byte) (uint64, error) {
	if len(data) < dx11HeaderSize {
		return 0, fmt.Errorf("arquivo de %d bytes não cabe no header DX11 (%d)", len(data), dx11HeaderSize)
	}
	if !IsPhyre(data) {
		return 0, fmt.Errorf("não é um .dds.phyre DX11 válido (magic/size/platformId)")
	}

	nsSize := uint64(mustLE32(data, offNamespaceSize))
	instCount := uint64(mustLE32(data, offInstanceListCount))
	userFixupCount := uint64(mustLE32(data, offUserFixupCount))
	userFixupDataSize := uint64(mustLE32(data, offUserFixupDataSize))
	totalDataSize := uint64(mustLE32(data, offTotalDataSize))

	off := uint64(dx11HeaderSize) + nsSize +
		instCount*instanceDescLen +
		totalDataSize +
		userFixupDataSize +
		userFixupCount*userFixupLen +
		uint64(mustLE32(data, offPointerArrayFixupSz)) +
		uint64(mustLE32(data, offPointerFixupSize)) +
		uint64(mustLE32(data, offArrayFixupSize))
	if off > uint64(len(data)) {
		return 0, fmt.Errorf("dataOffset %d além do fim do arquivo (%d)", off, len(data))
	}
	return off, nil
}

// textureMembers localiza no namespace os offsets de width/height (em
// PTexture2DBase) e de mips/flags (em PTextureCommonBase).
type textureMemberOffsets struct {
	width          uint64
	height         uint64
	mipmapCount    uint64
	maxMipmapLevel uint64
	textureFlags   uint64
}

func (n *namespace) textureMembers() (textureMemberOffsets, error) {
	var out textureMemberOffsets

	base, baseCount, ok := n.findClass("PTexture2DBase")
	if !ok {
		return out, fmt.Errorf("dds.phyre: classe PTexture2DBase não encontrada (sem width/height)")
	}
	common_, commonCount, ok := n.findClass("PTextureCommonBase")
	if !ok {
		return out, fmt.Errorf("dds.phyre: classe PTextureCommonBase não encontrada (sem info de mipmap)")
	}

	for _, want := range []struct {
		name string
		dst  *uint64
	}{
		{"m_height", &out.height},
		{"m_width", &out.width},
	} {
		off, err := n.findMember(base, baseCount, want.name)
		if err != nil {
			return out, err
		}
		*want.dst = off
	}
	for _, want := range []struct {
		name string
		dst  *uint64
	}{
		{"m_mipmapCount", &out.mipmapCount},
		{"m_maxMipLevel", &out.maxMipmapLevel},
		{"m_textureFlags", &out.textureFlags},
	} {
		off, err := n.findMember(common_, commonCount, want.name)
		if err != nil {
			return out, err
		}
		*want.dst = off
	}
	if out.width == 0 && out.height == 0 {
		return out, fmt.Errorf("dds.phyre: offsets de width/height não resolvidos")
	}
	return out, nil
}

// ExtractToDDS monta o arquivo DDS: header de 128 bytes preparado a partir
// dos metadados + payload bruto a partir de dataOffset (já é DDS).
func (t *Texture) ExtractToDDS() ([]byte, error) {
	header, err := prepareDDSHeader(t.Format, t.Width, t.Height, t.MipmapCount)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(header)+len(t.Payload))
	out = append(out, header...)
	out = append(out, t.Payload...)
	return out, nil
}

// PackDDS reempacota um DDS sobre o container original (port de
// convertDDS2Phyre), devolvendo o novo .dds.phyre completo.
//
// Restrição desta versão: o DDS precisa ter o MESMO formato do phyre — a
// realocação de fixups (_setTextureFormat) não é suportada ainda; nesse
// caso devolve erro claro em vez de corromper o arquivo.
func PackDDS(original, dds []byte) ([]byte, error) {
	t, err := Parse(original)
	if err != nil {
		return nil, err
	}
	if len(dds) < ddsHeaderLen {
		return nil, fmt.Errorf("dds: arquivo de %d bytes pequeno demais para um DDS (%d)", len(dds), ddsHeaderLen)
	}
	hdr, err := parseDDSHeader(dds)
	if err != nil {
		return nil, err
	}
	format, err := ddsFormat(hdr)
	if err != nil {
		return nil, err
	}
	if format != t.Format {
		return nil, fmt.Errorf(
			"mudança de formato não suportada: phyre é %q e o DDS é %q (esta versão só reempacota o mesmo formato)",
			t.Format, format,
		)
	}

	payload := dds[ddsHeaderLen:]
	out := make([]byte, 0, int(t.DataOffset)+len(payload))
	out = append(out, original[:int(t.DataOffset)]...)
	out = append(out, payload...)

	// maxTextureBufferSize do header DX11 (equivalente ao do tool C++).
	put32(out, offMaxTextureBufSize, bufferSizeByFormat(format, hdr.width, hdr.height))

	// Contagem de mips: DDS conta a textura base como mipmap, phyre não.
	fixedMips := hdr.mips
	if fixedMips > 1 {
		fixedMips--
	} else {
		fixedMips = 0
	}
	put32(out, t.InfoOffset+t.widthOffset, hdr.width)
	put32(out, t.InfoOffset+t.heightOffset, hdr.height)
	put32(out, t.InfoOffset+t.mipmapCountOffset, fixedMips)
	put32(out, t.InfoOffset+t.maxMipmapLevelOffs, fixedMips)
	// textureFlags fica intacto (mesmo valor) — não é reescrito.

	return out, nil
}

// String implementa fmt.Stringer para diagnóstico/log.
func (t *Texture) String() string {
	return fmt.Sprintf("%s %dx%d mips=%d (dataOffset=%d)",
		t.Format, t.Width, t.Height, t.MipmapCount, t.DataOffset)
}

// StemOf devolve o id da textura (caminho sem sufixo) a partir de um
// caminho relativo tipo ffx_data/gamedata/.../4660640_256_8.dds.phyre.
func StemOf(rel string) string {
	s := filepath.ToSlash(rel)
	s = strings.TrimPrefix(s, "./")
	return strings.TrimSuffix(s, Suffix)
}

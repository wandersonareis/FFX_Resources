// Dicionário de exibição (frontend decide a apresentação).
// Backend envia apenas sumário canônico {id, key}; os labels PT-BR vivem aqui.
// Nomes dos GRUPOS de events (fragmento do eventID) vivem em event-group-names.ts.

export type EntryKind =
  | 'events'
  | 'objects'
  | 'macro'
  | 'lockit'
  | 'help'
  | 'battletext'
  | 'cloud'
  | 'tutorial'
  | 'menumain'
  | 'images';

export const KIND_LABELS: Record<EntryKind, string> = {
  events: 'Eventos',
  objects: 'Sistema',
  macro: 'Dicionário',
  lockit: 'Loc Kit',
  help: 'Painel de ajuda',
  battletext: 'Textos de batalha',
  cloud: 'Cloud',
  tutorial: 'Tutorial',
  menumain: 'Texto menu principal',
  images: 'Imagens',
};

/** Tipo de armazenamento de um registro do lockit. */
export const LOCKIT_LABELS: Record<string, string> = {
  game: 'Jogo',
  utf8: 'UTF-8',
};

/** Collection id do lockit -> nome de exibição. */
export const LOCKIT_ENTRY_LABELS: Record<string, string> = {
  ffx_loc_kit_ps3: 'Kit de localização (FFX)',
  ffx2_loc_kit_ps3: 'Kit de localização (FFX-2)',
};

/** Collection id (basename sem extensão) -> nome de exibição. */
export const OBJECTS_LABELS: Record<string, string> = {
  important: 'Itens importantes',
  command: 'Comandos',
  item: 'Itens',
  a_ability: 'Habilidades',
  arms_txt: 'Armas',
  config_txt: 'Configurações',
  item_txt: 'Itens (textos)',
  mmain_txt: 'Menu principal',
  ply_rom: 'Quarto do jogador',
  btl_txt: 'Textos de batalha',
  btlend_txt: 'Fim de batalha',
  monmagic1: 'Magias de monstros 1',
  monmagic2: 'Magias de monstros 2',
  monster1: 'Monstros 1',
  monster2: 'Monstros 2',
  monster3: 'Monstros 3',
  build_txt: 'Construção',
  name_txt: 'Nomes',
  panel: 'Painel',
  sphere: 'Sphere Grid',
  save_txt: 'Save',
  status_txt: 'Status',
  summon_txt: 'Summons',
  w_name: 'Nomes de armas',
  accessory: 'Acessórios',
  job: 'Jobs',
  menu_txt: 'Menu',
  monmagic: 'Magias de monstros',
  monster: 'Monstros',
  oversoul: 'Oversoul',
  plate: 'Placas',
  ply_save: 'Save do jogador',
  lm_accesary: 'Acessórios (Last Mission)',
  lm_command: 'Comandos (Last Mission)',
  lm_dress: 'Dresspheres (Last Mission)',
  lm_floorname: 'Nomes de andares (Last Mission)',
  lm_item: 'Itens (Last Mission)',
  lm_mes: 'Mensagens (Last Mission)',
  lm_monmagic: 'Magias de monstros (Last Mission)',
  lm_monster: 'Monstros (Last Mission)',
  lm_player: 'Jogador (Last Mission)',
  lm_trap: 'Armadilhas (Last Mission)',
  lm_warehouse: 'Depósito (Last Mission)',
};

/** Collection id (stem do .sps2) dos painéis de ajuda -> nome de exibição. */
export const HELP_ENTRY_LABELS: Record<string, string> = {
  now_help: 'Ajuda (textos)',
  now_help_page: 'Ajuda (páginas)',
  mon_boku: 'Registro de monstros',
  s_monitor: 'Monitor de status',
  dvdcopy: 'Cópia de DVD',
  dvdcopy_page: 'Cópia de DVD (páginas)',
};

/** Prefixo da raiz da árvore de texturas (o resto vira a categoria). */
export const IMAGE_TREE_PREFIX = 'gamedata/ps3data/';

/**
 * Códigos de idioma que as pastas da árvore de texturas usam (ps3data):
 * `event/es`, `help_es`, `btl_ch`, `map_2d_ui_us`… `inter` (internacional)
 * não é idioma e fica de fora por não estar na lista.
 */
const IMAGE_LANG_CODES = new Set([
  'ch',
  'cn',
  'de',
  'es',
  'fr',
  'it',
  'jp',
  'kr',
  'sp',
  'us',
]);

/** Código de idioma → nome de exibição (PT-BR). */
export const LANGUAGE_LABELS: Record<string, string> = {
  ch: 'Chinês',
  cn: 'Chinês',
  de: 'Alemão',
  es: 'Espanhol',
  fr: 'Francês',
  it: 'Italiano',
  jp: 'Japonês',
  kr: 'Coreano',
  sp: 'Espanhol',
  us: 'Inglês',
};

/**
 * Idioma da PASTA onde a textura está — só para a linha "Idioma" do painel
 * de informações (cosmético: o valor nunca volta para o backend nem para o
 * encode).
 *
 * Fonte preferida é o caminho interno embutido no container
 * (`ImageEntry.assetPath`, ex.: `PS3Data/help_es/helpgra/help10.dds`), que
 * é o caminho que o jogo lê na leitura e vale até para um .dds.phyre
 * avulso, aberto sem árvore de diretórios; sem ele (container raro, que
 * só traz o basename), cai no id da árvore.
 *
 * Em qualquer das fontes: ignora o arquivo (último segmento — o `d3d11` do
 * id tem 5 letras e nunca casa) e aceita o código quando a pasta É o
 * código (`event/es`) ou termina com `_XX` (`help_es`, `btl_ch`). O `_`
 * que separa deixa de fora nomes só parecidos: `azit`, `newkit`,
 * `images`, `mon_boku`, `help_inter`.
 *
 * Devolve o código ("es") ou null quando o caminho não declara idioma —
 * aí a linha nem aparece.
 */
export function imageLanguage(
  assetPath?: string | null,
  id?: string | null
): string | null {
  for (const source of [assetPath, id]) {
    if (!source) continue;
    const found = languageOfPath(source);
    if (found) return found;
  }
  return null;
}

/** Varre as PASTAS de um caminho (assetPath ou id) atrás do código. */
function languageOfPath(path: string): string | null {
  const segments = path.split(/[\\/]/);
  for (const segment of segments.slice(0, -1)) {
    if (segment.length === 2 && IMAGE_LANG_CODES.has(segment)) {
      return segment;
    }
    const tail = segment.slice(-2);
    if (segment.endsWith(`_${tail}`) && IMAGE_LANG_CODES.has(tail)) {
      return tail;
    }
  }
  return null;
}

export function resolveEntryLabel(kind: EntryKind, id: string): string {
  if (kind === 'objects') {
    return OBJECTS_LABELS[id] ?? id;
  }
  if (kind === 'macro') {
    const m = /^chunk_(\d+)$/.exec(id);
    if (m) return `Bloco ${m[1]}`;
    return id;
  }
  if (kind === 'lockit') {
    return LOCKIT_ENTRY_LABELS[id] ?? id;
  }
  if (kind === 'help') {
    return HELP_ENTRY_LABELS[id] ?? id;
  }
  if (kind === 'images') {
    // Id é o caminho da textura; o header mostra o caminho relativo à raiz
    // da versão (a árvore lateral agrupa por categoria/diretório).
    return id.startsWith(IMAGE_TREE_PREFIX) ? id.slice(IMAGE_TREE_PREFIX.length) : id;
  }
  return id;
}

/** Rótulo do tipo (game/utf8) das rows do lockit. */
export function resolveSegmentLabel(name: string | undefined): string {
  if (!name) return '';
  return LOCKIT_LABELS[name] ?? name;
}

/** De onde veio a imagem servida (o backend decide, por preferência). */
export const IMAGE_SOURCE_LABELS: Record<string, string> = {
  dds: '.dds em disco',
  png: '.png em disco',
  phyre: 'decodificado do .dds.phyre',
};

/**
 * Rótulo de estado de uma cópia, relativo à textura de referência:
 *
 *	idêntica  → ainda sincronizada com o que está na tela;
 *	importada → editada em separado (divergente — sobrescrita por replicar);
 *	pristine  → nunca tocada (mas diferente da imagem de referência).
 */
export function copyState(d: { identical: boolean; modded: boolean }): string {
  if (d.identical) return 'idêntica';
  return d.modded ? 'importada' : 'pristine';
}

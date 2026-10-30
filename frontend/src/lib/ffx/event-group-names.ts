// Nomes de exibição dos GRUPOS de events (fragmento eventID[:4]).
// O backend envia só o id canônico; o label mora aqui (view decide).
//
// Chaves = os 4 primeiros caracteres do eventID, que é o nome da pasta em
// event/obj_ps3/<xx>/<id>/<id>.bin (azit0000 → "azit"; dream0000 → "drea").
// O valor é o LOCAL REAL e vale para TODAS as versões: o casamento é pelo
// fragmento EXATO, nunca por prefixo. Quem não tem nome cai no fallback de
// resolveEventGroup — primeiro fragmento nomeado do mesmo shortened (azmm
// entra em azit), depois o nome legado de 2 letras, senão o fragmento cru
// (ex.: isho). Troque pelo nome verdadeiro quando souber.
// Este arquivo é o único dono dos nomes de grupo, para não poluir o resto.

import type { GameVersionId } from './game-version';

/** Fragmento do eventID: os 4 primeiros caracteres (azit0000 → "azit"). */
export function shortenedOf(id: string): string {
  return id.slice(0, 4).toLowerCase();
}

/** FFX (ffx/event/obj_ps3/<xx>/<id>/<id>.bin). */
const FFX_GROUPS: Record<string, string> = {
  // Locais do jogo: nome real do local. O sufixo "(xx)" do diretório pai é
  // montado no rótulo, não aqui.
  azit: 'Home',
  bika: 'Bikanel Island',
  bjyt: 'Baaj Temple',
  bltz: 'Luca — Blitzball Stadium',
  bsil: 'Besaid Island',
  bsmm: 'Besaid Island — Flashback',
  bsvr: 'Besaid Village',
  bvyt: 'Besaid Temple',
  cdsp: 'Salvage Ship & Underwater Ruins',
  djyt: 'Djose Temple',
  dome: 'Zanarkand Dome',
  // A pasta é dream#### e shortenedOf corta em 4 → a chave exata é `drea`.
  drea: 'Dream Zanarkand',
  genk: 'Moonflow',
  grid: 'Sphere Grid',
  guad: 'Guadosalam',
  hiku: 'Fahrenheit — Airship & World Map',
  ikai: 'Farplane',
  kami: 'Thunder Plains',
  kino: 'Mushroom Rock Road',
  klyt: 'Kilika Woods & Kilika Temple',
  lchb: 'Luca',
  lmyt: 'Remiem Temple',
  luca: 'Luca',
  maca: 'Lake Macalania',
  mcfr: 'Macalania Woods',
  mcyt: 'Macalania Temple',
  mihn: "Mi'hen Highroad",
  // Sem local conhecido (lembrança de Jyscal e da mãe de Seymour): mostra só
  // o diretório pai, sem inventar nome — o rótulo omite o "(mm)" redundante.
  mmmc: 'mm',
  msmm: 'Via Purifico',
  mtgz: 'Mt. Gagazet, Mountain Cave & Zanarkand',
  nagi: 'Calm Lands & Cavern of the Stolen Fayth',
  omeg: 'Omega Ruins',
  ptkl: 'Kilika Port',
  sins: 'Inside Sin',
  slik: 'S.S. Liki',
  ssbt: 'Fahrenheit — Airship Model',
  stbv: 'Bevelle & Via Purifico',
  swin: 'S.S. Winno',
  titl: 'Main Menu',
  zkrn: 'Zanarkand',
  znkd: 'Dream Zanarkand',
  zzzz: 'Unknown',

  // Pastas utilitárias (sufixo não é código de local).
  cred: 'Créditos',
  game: 'Game Over',
  loop: 'Demo',
  open: 'Abertura',
  samp: 'Amostra',
  sysf: 'Sistema',
  test: 'Teste',
  // scen (Cenas/Eternal Calm, sc/scene*.bin) fica oculto na listagem do
  // ffx: aparece na aba eternalcalm.
};

/**
 * FFX-2 (ffx2/event/obj_ps3/<fragmento>/...). Usa os MESMOS fragmentos do FFX
 * (hiku, stbv, ikai, klyt...) + os específicos abaixo. O grupo "lmhi" é o
 * conteúdo da Last Mission: fica oculto na listagem do ffx2 e aparece na aba
 * lastmiss.
 */
const FFX2_EXTRA: Record<string, string> = {
  dnfr: 'Dungeon', // TODO: confirmar (dnfr####)
  lmhi: 'Last Mission', // lmhiku####
  spdn: 'Esferas', // TODO: confirmar (spdn*/spheresel)
  sphe: 'Esferas', // TODO: confirmar
};
const FFX2_GROUPS: Record<string, string> = { ...FFX_GROUPS, ...FFX2_EXTRA };

// eternalcalm é expansão do ffx e lê a MESMA árvore de events: os nomes de
// locais valem igual; o grupo "scen" (sc/scene*.bin — os arquivos do Eternal
// Calm) entra em cena nesta aba.
const ETERNAL_CALM_GROUPS: Record<string, string> = {
  ...FFX_GROUPS,
  scen: 'Eternal Calm',
};

// lastmiss é expansão do ffx2 e lê a MESMA árvore de events.
const GROUPS_BY_VERSION: Record<GameVersionId, Record<string, string>> = {
  ffx: FFX_GROUPS,
  eternalcalm: ETERNAL_CALM_GROUPS,
  ffx2: FFX2_GROUPS,
  lastmiss: FFX2_GROUPS,
};

/**
 * Nomes legados por shortened (2 letras): grupos do FFX-2 que tinham nome mas
 * cujo fragmento exato é desconhecido. Só entra em cena quando o shortened não
 * tem nenhum fragmento nomeado.
 */
const LEGACY_SHORT_GROUPS: Record<string, string> = {
  au: 'Esfera do Auron',
  ca: 'Cartas',
  en: 'Final',
  ev: 'Evento',
  pa: 'Esfera da Dor',
  sa: 'Sabotagem',
};

export interface EventGroupResolution {
  /** Fragmento do nó alvo (onde os arquivos deste grupo moram). */
  target: string;
  /** Nome do local; vazio quando não nomeado (o nó exibe o próprio fragmento). */
  name: string;
}

/**
 * Resolve o nó de um fragmento de eventID:
 * - fragmento nomeado → nó próprio com o nome;
 * - sem nome → entra no PRIMEIRO fragmento nomeado do mesmo shortened
 *   (2 letras, ordem alfabética): azmm → azit (Home), bsyt → bsil
 *   (Besaid Island);
 * - nenhum nomeado no shortened → nome legado do shortened (Cartas...);
 * - senão → nó anônimo exibindo o próprio fragmento, igual ao nome da pasta
 *   no disco, para ser achado na busca da árvore de arquivos.
 */
export function resolveEventGroup(
  version: GameVersionId,
  fragment: string,
  siblings: string[],
): EventGroupResolution {
  const names = GROUPS_BY_VERSION[version] ?? {};
  const self = names[fragment];
  if (self) return { target: fragment, name: self };

  const namedSibling = siblings
    .filter(
      (s) =>
        s !== fragment &&
        // Só o MESMO shortened (2 letras): sem isto, o sort() abaixo pega o
        // menor fragmento nomeado da ÁRVORE INTEIRA — como "azit" é o menor
        // global, todo órfão de qualquer pasta caía em Home.
        s.slice(0, 2) === fragment.slice(0, 2) &&
        names[s],
    )
    .sort()[0];
  if (namedSibling) return { target: namedSibling, name: names[namedSibling] };

  const legacy = LEGACY_SHORT_GROUPS[fragment.slice(0, 2)];
  if (legacy) return { target: fragment, name: legacy };

  return { target: fragment, name: '' };
}

/**
 * Rótulo do nó de um grupo: `Nome (xx) - N`, em que `xx` é o diretório pai
 * (2 primeiras letras do fragmento). Quando o próprio nome É esse código
 * (mmmc → "mm", local sem nome conhecido), o sufixo seria redundante e sai.
 * Sem nome → o fragmento cru, igual à pasta no disco (aí a busca encontra).
 */
export function formatEventGroupLabel(name: string, target: string, count: number): string {
  if (!name) return `${target} - ${count}`;
  const parent = target.slice(0, 2);
  return name === parent ? `${name} - ${count}` : `${name} (${parent}) - ${count}`;
}

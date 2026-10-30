const LAYOUT_KEY = 'ffx-resources.layout';

/**
 * Validade da config persistida: um layout guardado há mais de 30 dias não
 * deve sobreviver a mudanças de layout do app — na leitura a chave é apagada
 * e valem os padrões atuais.
 */
export const LAYOUT_MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000;

/**
 * Padrões em PIXELS: o `react-resizable-panels` interpreta número como px, e
 * são as larguras que o app já usava antes de virarem redimensionáveis
 * (sidebar 290px; coluna de informações de imagem 20rem).
 *
 * O que é PERSISTIDO vem em percentual (0..100) do grupo — a unidade que
 * `onLayoutChanged` entrega — porque ela sobrevive a janelas de larguras
 * diferentes sem empurrar os painéis para fora.
 */
export const LAYOUT_DEFAULTS_PX = {
  sidebar: 290,
  imageInfo: 320,
} as const;

/** Mapa id do painel -> tamanho em percentual do grupo (0..100). */
export type PanelLayout = Record<string, number>;

export interface LayoutConfig {
  sidebar: PanelLayout | null;
  imageInfo: PanelLayout | null;
  /** Epoch ms do último redimensionamento feito pelo usuário. */
  savedAt: number;
}

function isPanelLayout(value: unknown): value is PanelLayout {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false;
  }
  const sizes = Object.values(value as Record<string, unknown>);
  if (sizes.length === 0) return false;
  return sizes.every(
    (size) => typeof size === 'number' && Number.isFinite(size) && size > 0 && size <= 100,
  );
}

/**
 * Valida o JSON persistido. Devolve null para entrada ausente, malformada ou
 * com `savedAt` vencido (> 30 dias) — nos dois últimos casos o chamador deve
 * descartar a chave e cair nos padrões atuais.
 */
export function parseLayoutConfig(raw: string | null, now: number): LayoutConfig | null {
  if (raw === null) return null;

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }

  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    return null;
  }

  const config = parsed as Partial<LayoutConfig>;
  if (typeof config.savedAt !== 'number' || !Number.isFinite(config.savedAt)) {
    return null;
  }
  // Janela SIMÉTRICA de ±30 dias: além para trás é config velha; além para
  // frente é relógio andado (ou timestamp corrompido) — sem isto um savedAt
  // no futuro deixaria a expiração de nunca disparar.
  if (Math.abs(now - config.savedAt) > LAYOUT_MAX_AGE_MS) return null;

  return {
    sidebar: isPanelLayout(config.sidebar) ? config.sidebar : null,
    imageInfo: isPanelLayout(config.imageInfo) ? config.imageInfo : null,
    savedAt: config.savedAt,
  };
}

/**
 * Persistência do layout dos grupos redimensionáveis (sidebar e informações
 * de imagem), no localStorage. A chave é única: guardar o sidebar preserva a
 * coluna de imagem e vice-versa.
 *
 * Sem botão de reset: apagar a chave no localStorage é o único caminho de
 * volta ao padrão (a leitura também a apaga sozinha quando vencida).
 */
export const layoutStorage = {
  load(now: number = Date.now()): LayoutConfig | null {
    try {
      const raw = localStorage.getItem(LAYOUT_KEY);
      const config = parseLayoutConfig(raw, now);
      // Inválido ou vencido: remove para não revalidar a cada montagem.
      if (config === null && raw !== null) localStorage.removeItem(LAYOUT_KEY);
      return config;
    } catch {
      // localStorage indisponível: segue só com os padrões.
      return null;
    }
  },

  /** Grava o layout de UM grupo, preservando o outro e renovando o savedAt. */
  saveGroup(group: 'sidebar' | 'imageInfo', layout: PanelLayout, now: number = Date.now()): void {
    if (!isPanelLayout(layout)) return;
    try {
      const previous = layoutStorage.load(now);
      const next: LayoutConfig = {
        sidebar: previous?.sidebar ?? null,
        imageInfo: previous?.imageInfo ?? null,
        [group]: layout,
        savedAt: now,
      };
      localStorage.setItem(LAYOUT_KEY, JSON.stringify(next));
    } catch {
      // localStorage indisponível: ignora.
    }
  },
};

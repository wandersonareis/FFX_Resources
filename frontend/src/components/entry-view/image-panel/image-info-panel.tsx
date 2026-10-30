'use client';

import { Copy } from 'lucide-react';
import type { ReactNode } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import { Badge } from '@/components/ui/badge';
import { formatBytes } from '@/lib/ffx/bytes';
import {
  copyState,
  IMAGE_SOURCE_LABELS,
  imageLanguage,
  LANGUAGE_LABELS,
  resolveEntryLabel,
} from '@/lib/ffx/display-names';

/**
 * Coluna de informações da textura aberta: metadados do container, estado
 * (pristine / importada), a linha "Idioma" (cosmético — código da pasta,
 * lido do caminho interno embutido quando existe), a linha "Repetidas"
 * (grupo de cópias de mesma imagem — otimização do DVD) com atalho de
 * navegação para cada uma, e os caminhos dos derivados já extraídos.
 */
export function ImageInfoPanel({
  image,
  duplicates,
  onGoToCopy,
}: {
  image: dto.ImageEntry;
  duplicates: dto.ImageDuplicate[];
  /** Navega para uma cópia (a árvore já tem o entry — é só selecionar). */
  onGoToCopy: (id: string, key: string, vbfPath?: string) => void;
}) {
  // Linha "Repetidas": o grupo de cópias de mesma imagem (otimização do
  // DVD) com atalho de navegação para cada uma delas.
  const repeatsRow: [label: string, value: ReactNode] =
    duplicates.length > 0
      ? [
          'Repetidas',
          <div key="dups" className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge className="border border-warning/40 bg-warning/15 text-warning">
                <Copy size={12} className="mr-1" />×{duplicates.length + 1} idênticas
              </Badge>
              <span className="text-xs opacity-70">
                {formatBytes(image.dupPayload * duplicates.length)} de cópias
              </span>
            </div>
            <ul className="space-y-0.5">
              {duplicates.map((d) => (
                <li key={d.id}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left text-xs hover:bg-muted"
                    title={`${d.id} — abrir esta cópia`}
                    onClick={() => void onGoToCopy(d.id, d.key, d.vbfPath)}
                  >
                    <span className="min-w-0 flex-1 truncate underline-offset-2 hover:underline">
                      {resolveEntryLabel('images', d.id)}
                    </span>
                    <Badge
                      variant={d.identical ? 'outline' : 'secondary'}
                      className="shrink-0 text-[10px]"
                    >
                      {copyState(d)}
                    </Badge>
                  </button>
                </li>
              ))}
            </ul>
          </div>,
        ]
      : [
          'Repetidas',
          <span key="dups" className="opacity-70">
            nenhuma (imagem única)
          </span>,
        ];

  // Linha "Idioma" (cosmético): o código da pasta em que a textura está,
  // lido do caminho interno embutido no container (assetPath) com fallback
  // para o id da árvore. Sem idioma no caminho, a linha NÃO existe.
  const lang = imageLanguage(image.assetPath, image.metadata.id);
  const langRows: Array<[label: string, value: ReactNode]> = lang
    ? [
        [
          'Idioma',
          <Badge key="lang" variant="outline">
            {lang} — {LANGUAGE_LABELS[lang] ?? lang}
          </Badge>,
        ],
      ]
    : [];

  const meta: Array<[label: string, value: ReactNode]> = [
    ['Formato', image.format],
    ['Dimensões', `${image.width}×${image.height}`],
    [
      'Mipmaps',
      `${image.mipmapCount}${image.maxMipmapLevel ? ` (max ${image.maxMipmapLevel})` : ''}`,
    ],
    [
      'Fonte',
      <Badge key="source" variant="outline">
        {IMAGE_SOURCE_LABELS[image.source] ?? image.source}
      </Badge>,
    ],
    [
      'Estado',
      image.modded ? (
        <Badge key="modded" className="border border-success/40 bg-success/15 text-success">
          importada (mods/)
        </Badge>
      ) : (
        <Badge key="modded" variant="outline">
          pristine (data/)
        </Badge>
      ),
    ],
    ...langRows,
    repeatsRow,
    [
      'Cópias extraídas',
      image.ddsPath || image.pngPath ? (
        <span key="paths" className="block break-all opacity-80">
          {image.ddsPath ? <span className="block">{image.ddsPath}</span> : null}
          {image.pngPath ? <span className="block">{image.pngPath}</span> : null}
        </span>
      ) : (
        'nenhuma'
      ),
    ],
  ];

  return (
    // O card da coluna é um RETRATO VISUAL absoluto (inset-0), não um wrapper:
    // em flow ele somaria borda e padding na altura medida do painel. Absoluto,
    // ele não participa da medição e ainda assim preenche a coluna INTEIRA
    // (a margem do p-1 visível por toda a altura). O containing block é o
    // wrapper do painel — o ResizablePanel id="info" recebe
    // className="relative" no index. O wrapper NUNCA rola (o dl mede exatamente
    // a altura dele): quem rola é o próprio dl — se o card rolasse junto do
    // conteúdo (o que acontecia com o wrapper como container de rolagem), o
    // conteúdo aparecia POR FORA do card ao rolar uma coluna alta.
    <>
      <div aria-hidden="true" className="absolute inset-0 rounded-md border bg-muted/40" />
      {/*
          O dl É o container de rolagem da coluna: h-full (a altura do
          painel) + overflow-y-auto. Conteúdo mais alto (repetidas +
          caminhos extraídos em tela baixa) rola DENTRO dele — o card fica
          parado, cobrindo sempre a parte visível. O p-1 afasta o conteúdo
          da borda do card nos quatro lados; o dl fica por cima do card
          (relative pinta depois do absoluto anterior).
      */}
      <dl className="relative h-full space-y-2 overflow-y-auto p-1 text-sm">
        {meta.map(([label, value]) => (
          <div key={label} className="flex flex-col gap-1">
            <dt className="text-xs uppercase tracking-wide opacity-60">{label}</dt>
            <dd className="min-w-0">{value}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}

'use client';

import { AlertTriangle } from 'lucide-react';
import { useEffect, useState } from 'react';
import type * as dto from '@/bindings/ffxresources/backend/dto/models';
import type { EntryView, ImageActionState } from '@/components/entry-view/entry-view-store';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { parseError } from '@/lib/ffx/error-handler';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';
import { imageDuplicates } from '@/lib/ffx/tree-data';

export type ActionProps = { view: EntryView; action: ImageActionState };

/**
 * Cópias do alvo da ação: vêm prontas quando o chamador as conhecia (menu já
 * buscou / painel carrega a textura); null = carregando. Só busca quando não
 * vieram — o binding é leve, mas para quê chamá-lo?
 *
 * A lista guarda o id a que pertence: se a ação trocar de textura com o
 * diálogo montado, a lista antiga não serve (volta a "carregando").
 */
export function useActionDuplicates(
  view: EntryView,
  action: ImageActionState,
): dto.ImageDuplicate[] | null {
  const [state, setState] = useState<{ id: string; list: dto.ImageDuplicate[] | null }>({
    id: action.id,
    list: action.duplicates,
  });

  useEffect(() => {
    // Veio pronta do chamador: nada a buscar.
    if (action.duplicates) return;
    // Lista desta textura já chegou (ou está a caminho).
    if (state.id === action.id && state.list !== null) return;
    let dead = false;
    imageDuplicates(action.id, view.version)
      .then((res) => {
        if (!dead) setState({ id: action.id, list: res.duplicates ?? [] });
      })
      .catch((error) => {
        if (dead) return;
        setState({ id: action.id, list: [] });
        toast.error(parseError(error));
      });
    return () => {
      dead = true;
    };
  }, [action.id, action.duplicates, state.id, state.list, view.version]);

  if (state.id !== action.id) return action.duplicates;
  return action.duplicates ?? state.list;
}

/** Toast de uma operação em lote (extrair / replicar / deletar). */
export function reportBatch(res: dto.BatchResult, singular: string, plural: string): void {
  // Mensagem na ordem "4 texturas apagadas.": a contagem vem antes e o
  // participio concorda com ela (a ordem antiga, "Apagadas 4 texturas.",
  // ficava invertida).
  const done = res.done.length;
  const form = (n: number, one: string, many: string) => (n === 1 ? one : many);
  if (res.failed.length > 0) {
    // Com falha parcial o substantivo é o total (quem falhou aparece na
    // descrição): "3 de 4 texturas apagadas."
    toast.warning(
      `${done} de ${res.total} ${form(res.total, 'textura', 'texturas')} ` +
        `${form(res.total, singular, plural).toLowerCase()}.`,
      {
        description: res.failed.slice(0, 3).join('; '),
      },
    );
    return;
  }
  toast.success(
    `${done} ${form(done, 'textura', 'texturas')} ` +
      `${form(done, singular, plural).toLowerCase()}.`,
  );
}

/** Escopos do delete: onde os containers são apagados. */
export type DeleteScope = 'both' | 'data' | 'mods';

export const DELETE_SCOPES: Array<{
  value: DeleteScope;
  title: string;
  hint: string;
}> = [
  {
    value: 'both',
    title: 'Ambos (data/ e mods/)',
    hint: 'apaga o original e a substituição — a textura sai da árvore',
  },
  {
    value: 'data',
    title: 'Só o original (data/)',
    hint: 'a textura sai da árvore; mods/ permanece como está',
  },
  {
    value: 'mods',
    title: 'Só em mods/',
    hint: 'desfaz a substituição e volta ao conteúdo original',
  },
];

/**
 * Aviso do alerta, em duas partes: a primeira é fixa (por que não há
 * desfazer) e a segunda muda com o escopo selecionado.
 *
 * Os textos espelham o que `ddsphyre.Delete` realmente remove: o container
 * `.dds.phyre` de data/ e/ou de mods/ conforme o escopo, e SEMPRE os
 * derivados `.dds`/`.png` de mods/images/ — por isso os derivados
 * aparecem nos três itens.
 */
const DELETE_IRREVERSIBLE =
  'Os arquivos são removidos do disco, fora da lixeira, e não há ' + 'como desfazer.';

const DELETE_SCOPE_NOTICES: Record<DeleteScope, string> = {
  data:
    'Sai da árvore o binário .dds.phyre original em data/. Os derivados ' +
    '.dds e .png em mods/images/ também são apagados, e a ' +
    'substituição em mods/ permanece.',
  mods:
    'Sai a substituição .dds.phyre em mods/, junto com os derivados ' +
    '.dds e .png em mods/images/. O binário original em data/ ' +
    'permanece e a textura volta ao conteúdo original.',
  both:
    'Sai dos dois lados: o binário .dds.phyre original em data/ E a ' +
    'substituição .dds.phyre em mods/, além dos derivados .dds e .png ' +
    'em mods/images/. A textura sai da árvore por completo.',
};

/** Alerta destrutivo cuja descrição acompanha o escopo escolhido. */
export function DeleteIrreversibleAlert({ scope }: { scope: DeleteScope }) {
  return (
    <Alert variant="destructive">
      <AlertTriangle />
      <AlertTitle>Irreversível</AlertTitle>
      <AlertDescription className="gap-2">
        <p>{DELETE_IRREVERSIBLE}</p>
        <p className="font-medium">{DELETE_SCOPE_NOTICES[scope]}</p>
      </AlertDescription>
    </Alert>
  );
}

/**
 * Os três escopos como rádio, com o rótulo acima. Idêntico nos dois
 * diálogos de delete (o `name` difere para não colidir quando os dois
 * montam, e a legenda muda com o alvo — uma textura ou a seleção).
 */
export function DeleteScopeField({
  scope,
  onChange,
  legend,
  name,
}: {
  scope: DeleteScope;
  onChange: (scope: DeleteScope) => void;
  legend: string;
  name: string;
}) {
  return (
    <fieldset className="space-y-1.5">
      <legend className="mb-1 text-xs uppercase tracking-wide opacity-60">{legend}</legend>
      {DELETE_SCOPES.map((option) => (
        <label
          key={option.value}
          className={`flex cursor-pointer items-start gap-2 rounded border p-2 text-sm ${
            scope === option.value ? 'border-primary bg-primary/5' : 'border-border'
          }`}
        >
          <input
            type="radio"
            name={name}
            className="mt-0.5"
            checked={scope === option.value}
            onChange={() => onChange(option.value)}
          />
          <span className="min-w-0">
            <span className="block font-medium">{option.title}</span>
            <span className="block text-xs opacity-70">{option.hint}</span>
          </span>
        </label>
      ))}
    </fieldset>
  );
}

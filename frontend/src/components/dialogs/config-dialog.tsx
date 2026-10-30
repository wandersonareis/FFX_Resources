'use client';

import { FolderOpen } from 'lucide-react';
import { useRef, useState } from 'react';
import { HasUnsavedEdits } from '@/bindings/ffxresources/app';
import {
  ResolveEnableModsChange,
  SelectDirectory,
  SelectGameExeFile,
} from '@/bindings/ffxresources/configservice';
import {
  type ModsGuardDecision,
  ModsToggleGuardDialog,
} from '@/components/dialogs/mods-toggle-guard-dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  commitEnableMods,
  commitGameExeLocation,
  commitGameLocation,
  commitTranslateLocation,
  useAppConfig,
} from '@/lib/ffx/config-store';
import { editDraft } from '@/lib/ffx/edit-draft';
import { loggedToast as toast } from '@/lib/ffx/toast-logged';

type FieldKey = 'gameExe' | 'gameFiles' | 'translate';
type DirectoryKey = Extract<FieldKey, 'gameFiles' | 'translate'>;

interface DirectoryField {
  key: DirectoryKey;
  label: string;
  hint: string;
  dialogTitle: string;
}

/** Commits por campo — actions do store (otimista + eco do backend). */
const COMMIT: Record<DirectoryKey, (path: string) => Promise<void>> = {
  gameFiles: commitGameLocation,
  translate: commitTranslateLocation,
};

const DIRECTORY_FIELDS: DirectoryField[] = [
  {
    key: 'gameFiles',
    label: 'Arquivos originais do jogo',
    hint: 'Padrão: <execução>/data',
    dialogTitle: 'Selecione a pasta dos arquivos originais do jogo',
  },
  {
    key: 'translate',
    label: 'Arquivos traduzidos (reimport)',
    hint: 'Padrão: <jogo>/mods/translated',
    dialogTitle: 'Selecione a pasta dos arquivos traduzidos',
  },
];

/**
 * Par LITERAL aceito pelo backend (NormalizeGameExe: FFX.exe/FFX-2.exe,
 * sem curinga) — espelhado aqui só para feedback imediato na digitação;
 * a decisão final é sempre do eco: o backend recusa em silêncio e o
 * store devolve o input ao valor persistido.
 */
const GAME_EXE_NAMES = new Set(['ffx.exe', 'ffx-2.exe']);

/** Nome do arquivo de um caminho (Windows ou POSIX). */
const basename = (p: string): string => p.split(/[\\/]/).pop() ?? '';

/**
 * Diálogo de diretórios: apenas gamefiles e translated (fonte do
 * reimport, <game>/mods/translated por padrão). Extract/reimport são
 * internos do backend e não aparecem aqui.
 *
 * Os valores vêm do store de config (useAppConfig): o backend persiste
 * cada mudança no config.json e re-emite o snapshot completo (evento
 * AppConfigChanged), então o store — e os inputs com ele — refletem a
 * verdade NA HORA, inclusive a escolha do seletor nativo, sem fechar e
 * reabrir o modal.
 *
 * Os inputs são RASCUNHOS livres sobre o store: digitar, colar e apagar
 * só editam o draft local (nunca o backend por tecla); o commit acontece
 * no blur/Enter/seletor. Enquanto o campo tem foco o eco não o
 * sobrescreve — o draft protege o texto em edição.
 */
export function ConfigDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-160">
        {/* Header fica AQUI (radix DialogTitle exige o root Dialog); na
            janela filha o mesmo papel é do titlebar do WindowChrome. */}
        <DialogHeader>
          <DialogTitle>Configurações</DialogTitle>
          <DialogDescription className="sr-only">Diretórios usados pelo app</DialogDescription>
        </DialogHeader>
        <ConfigDialogContent />
      </DialogContent>
    </Dialog>
  );
}

/**
 * Corpo do diálogo — compartilhado pelo modal (browser/server mode) e
 * pela janela filha /w/#config: mesmos drafts, commits e validação.
 * Header é do INVÓLUCRO (modal: DialogHeader; janela: titlebar do
 * WindowChrome) — aqui só o conteúdo.
 *
 * Mudanças commitam NA HORA (blur/Enter/seletor: otimista + eco
 * AppConfigChanged): não existe estado "não salvo", fechar é só navegar —
 * e um rascunho ainda em digitação é descartado ao fechar. EXCEÇÃO: o
 * checkbox de mods — com edições não salvas ele passa pela guarda
 * (ModsToggleGuardDialog) antes de aplicar, porque o toggle recarrega
 * todo o conteúdo e o rascunho sujo pede decisão explícita.
 */
export function ConfigDialogContent() {
  const config = useAppConfig();

  // Draft do campo em edição — só existe a partir da primeira tecla/colar
  // (o valor exibido é o draft quando há um, senão o valor do store).
  const [editing, setEditing] = useState<{ key: FieldKey; text: string } | null>(null);

  // Commits em voo (Enter ainda não resolvido no blur seguinte: o mesmo
  // caminho não é persistido duas vezes).
  const pending = useRef(new Set<FieldKey>());

  // Toggle de mods pendente na guarda de edições não salvas: o valor que o
  // usuário quer aplicar, null = guarda fechada. Enquanto a guarda decide,
  // NADA é commitado — o checkbox segue exibindo o estado do store (o
  // antigo), e o cancelamento o devolve intacto, sem tocar rascunhos nem
  // os inputs de caminho.
  const [pendingMods, setPendingMods] = useState<boolean | null>(null);

  const storeValue: Record<FieldKey, string> = {
    gameExe: config.gameExeLocation,
    gameFiles: config.gameFilesLocation,
    translate: config.translateLocation,
  };

  const shownValue = (key: FieldKey): string =>
    editing?.key === key ? editing.text : storeValue[key];

  const runCommit = (key: FieldKey, promise: Promise<void>): Promise<void> => {
    pending.current.add(key);
    return promise.finally(() => pending.current.delete(key));
  };

  /**
   * Commit de campo: direto para os actions do store (otimista + eco).
   * Regras por campo:
   *  - executável vazio é commit VÁLIDO (limpar a config desliga as
   *    raízes VBF da sidebar); caminho fora de FFX.exe/FFX-2.exe é
   *    recusado com toast e o campo volta ao persistido;
   *  - diretório vazio NÃO comita: no Enter avisa com toast, no blur
   *    reverte em silêncio ao valor do store (apagar/colar seguem
   *    livres durante a edição).
   */
  const commitField = (key: FieldKey, raw: string, viaEnter: boolean): void => {
    const path = (raw ?? '').trim();
    if (key === 'gameExe') {
      if (path !== '' && !GAME_EXE_NAMES.has(basename(path).toLowerCase())) {
        toast.error('Escolha um dos executáveis do jogo: FFX.exe ou FFX-2.exe.', {
          duration: 4000,
        });
        if (viaEnter) setEditing(null); // Enter: reverte ao persistido
        return; // blur reverte sozinho (editing já saiu)
      }
      void runCommit(key, commitGameExeLocation(path));
      return;
    }
    if (path === '') {
      if (viaEnter) {
        toast('Informe um diretório válido.', { duration: 3000 });
        setEditing(null); // reverte o campo vazio ao valor do store
      }
      return;
    }
    if (path === storeValue[key]) return;
    void runCommit(key, COMMIT[key](path));
  };

  const onFieldBlur = (key: FieldKey): void => {
    const draft = editing?.key === key ? editing.text : null;
    if (draft === null) return; // nunca foi editado: nada a commitar
    setEditing(null);
    if (pending.current.has(key)) return; // Enter ainda em voo
    commitField(key, draft, false);
  };

  const onFieldEnter = (key: FieldKey): void => {
    const draft = editing?.key === key ? editing.text : null;
    if (draft === null) return; // sem edição não há o que confirmar
    if (pending.current.has(key)) return;
    commitField(key, draft, true);
  };

  const selectDirectory = async (field: DirectoryField): Promise<void> => {
    try {
      const path = await SelectDirectory(field.dialogTitle);
      if (!path) return; // cancelado
      // A escolha substitui o rascunho digitado — o store assume o novo
      // caminho na hora (commit otimista) e o eco confirma.
      setEditing(null);
      void runCommit(field.key, COMMIT[field.key](path));
    } catch (error) {
      notify(error);
    }
  };

  const selectExe = async (): Promise<void> => {
    try {
      // "" = cancelado ou inválido (o seletor do backend já avisou).
      const path = await SelectGameExeFile();
      if (!path) return;
      setEditing(null);
      void runCommit('gameExe', commitGameExeLocation(path));
    } catch (error) {
      notify(error);
    }
  };

  /**
   * Alternar o checkbox de mods (#EnableMods — o "disable mods" do
   * config). Sem edições não salvas aplica direto (commit otimista + eco;
   * o eco invalida os caches do backend e as abas dão coldReload). Com
   * rascunho sujo, abre a guarda: aplicar o toggle recarrega todo o
   * conteúdo e a decisão de salvar/descartar é do usuário.
   *
   * O critério de "sujo" é o MESMO boolean que habilita o botão Salvar
   * da tabela (editDraft) — leitura local e exata na janela principal.
   * A janela FILHA de config (desktop) tem outro editDraft (vazio): lá
   * vale a flag do backend (HasUnsavedEdits), espelho da mesma fonte
   * mantido pelo GameVersionTab via SetUnsavedEdits.
   */
  const onToggleMods = async (next: boolean): Promise<void> => {
    if (next === config.enableMods) return;
    let dirty = editDraft.getSnapshot().hasDirty;
    if (!dirty) {
      dirty = await HasUnsavedEdits().catch(() => false);
    }
    if (!dirty) {
      void commitEnableMods(next);
      return;
    }
    setPendingMods(next);
  };

  /**
   * Decisão da guarda (Salvar/Descartar): viaja pelo evento
   * EnableModsChange e a janela PRINCIPAL — dona dos rascunhos — executa
   * (save/discard + apply). Se o salvar falhar lá, o toggle não é
   * aplicado e o checkbox permanece no estado antigo.
   */
  const resolveModsGuard = (decision: ModsGuardDecision): void => {
    const next = pendingMods;
    setPendingMods(null);
    if (next === null) return;
    void ResolveEnableModsChange(next, decision).catch(notify);
  };

  return (
    <>
      <div className="flex flex-col gap-3 pt-2 min-w-0">
        <div className="flex items-center gap-2 pt-2">
          <div className="grid flex-1 gap-1.5">
            <Label htmlFor="GameExeLocation">Executável do jogo</Label>
            <Input
              id="GameExeLocation"
              value={shownValue('gameExe')}
              placeholder="FFX.exe ou FFX-2.exe"
              onChange={(e) => setEditing({ key: 'gameExe', text: e.target.value })}
              onBlur={() => onFieldBlur('gameExe')}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  onFieldEnter('gameExe');
                }
              }}
            />
            <p className="text-xs text-muted-foreground">
              Dele saem as raízes dos containers <code>.vbf</code> (pasta do exe e pasta{' '}
              <code>data/</code>) exibidos no fim da árvore. Vazio desliga as raízes VBF.
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => void selectExe()}
            aria-label="Selecionar o executável do jogo"
            title="Procurar arquivo"
          >
            <FolderOpen size={20} />
          </Button>
        </div>
        {DIRECTORY_FIELDS.map((field) => (
          <div key={field.key} className="flex items-center gap-2">
            <div className="grid flex-1 gap-1.5">
              <Label htmlFor={field.key}>{field.label}</Label>
              <Input
                id={field.key}
                value={shownValue(field.key)}
                onChange={(e) => setEditing({ key: field.key, text: e.target.value })}
                onBlur={() => onFieldBlur(field.key)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    onFieldEnter(field.key);
                  }
                }}
              />
              <p className="text-xs text-muted-foreground">{field.hint}</p>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => void selectDirectory(field)}
              aria-label={`Selecionar ${field.label}`}
              title="Procurar pasta"
            >
              <FolderOpen size={20} />
            </Button>
          </div>
        ))}
      </div>

      <div className="flex items-start gap-2 pt-2">
        <Checkbox
          id="EnableMods"
          checked={config.enableMods}
          onCheckedChange={(v) => void onToggleMods(Boolean(v))}
        />
        <div className="grid gap-1 leading-none">
          <Label htmlFor="EnableMods" className="cursor-pointer">
            Preferir binários traduzidos (mods)
          </Label>
          <p className="text-xs text-muted-foreground">
            Ao carregar, usa primeiro os binários salvos em
            <code className="mx-1">mods/</code>e cai para os originais quando não existirem — a
            tradução continua de onde parou. O import de JSON/strings grava os binários nessa pasta.
          </p>
        </div>
      </div>

      <ModsToggleGuardDialog
        next={pendingMods}
        onDecision={resolveModsGuard}
        onCancel={() => setPendingMods(null)}
      />
    </>
  );
}

/**
 * Erros do seletor nativo: toast com cópia para o log (loggedToast.error
 * já escreve no log do backend via WriteLog — não emitir "Notify", que
 * circularia de volta ao ProgressNotifier).
 */
function notify(error: unknown): void {
  const message = error instanceof Error ? error.message : String(error ?? 'Erro');
  toast.error(message, { duration: 4000 });
}

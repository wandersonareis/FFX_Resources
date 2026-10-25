'use client';

import { memo } from 'react';
import {
  ChevronDown,
  ChevronRight,
  File as FileIcon,
  FileText,
  Folder,
  FolderOpen,
  Image as ImageIcon,
  Loader2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  vbfNodeCheckState,
  type VbfSelectionTreeNode,
} from '@/lib/ffx/vbf-selection';
import {
  EMPTY_IDS,
  entryCheckState,
  entryNodeId,
  TEXT_ROOT_NODE_ID,
  type SideNode,
} from '@/lib/ffx/content-tree/model';
import { useContentTree } from './tree-context';

/**
 * Item recursivo da árvore. `node` e `depth` são as ÚNICAS props: expansão,
 * seleção, checkboxes e ações vêm do contexto, então a recursão não repassa
 * nada adiante.
 *
 * MEMOIZADO: com o valor do contexto estável, um nó só re-renderiza quando o
 * PRÓPRIO nó muda (expansão/seleção/marcação) — mudanças de estado da view
 * que não tocam no nó passam reto.
 */
export const TreeItem = memo(function TreeItem({
  node,
  depth,
}: {
  node: SideNode;
  depth: number;
}) {
  const {
    expanded,
    selectedId,
    selectedByKind,
    vbfSelectionByRoot,
    onToggle,
    onSelect,
    onCheck,
  } = useContentTree();

  // Diretório do .vbf ainda sem filhos (ou grupo de macrodic) também é
  // expansível: o chevron aparece ANTES da listagem chegar.
  const hasChildren =
    (!!node.children && node.children.length > 0) || (node.expandable ?? false);
  const isExpanded = expanded.has(node.id);
  const isSelected = node.entry ? selectedId === entryNodeId(node.entry) : false;
  // O container .vbf é somente leitura, mas seus caminhos podem ser
  // selecionados para extração/exportação sem alterar o arquivo.
  const isVbf = node.vbf === true;
  // Raiz do container .vbf não tem checkbox: extrair o container inteiro
  // não faz sentido — marque diretórios/arquivos.
  const isVbfContainerRoot = isVbf && !node.entry && (node.vbfPath ?? '') === '';
  // Raiz de kind (Eventos/Sistema/Dicionário…) TEM checkbox tri-state:
  // marcar ela marca o kind inteiro (o backend deduplica para o artefato
  // bulk único). O agrupamento `Texto` e as raízes de imagem NÃO têm —
  // `Texto` é só visual (sem kind) e imagens seguem pela marcação de
  // folha/grupo, que alimenta extrair/deletar.
  const isTextRoot = node.id === TEXT_ROOT_NODE_ID || node.id.startsWith('kind:images');
  const selectable = !isVbfContainerRoot && !isTextRoot && !!node.kind;
  const vbfRoot = node.vbfRoot ?? node.entry?.vbf?.root;
  const vbfPath = node.vbfPath ?? node.entry?.vbf?.path ?? '';
  const selected = node.kind
    ? (selectedByKind.get(node.kind) ?? EMPTY_IDS)
    : EMPTY_IDS;

  let checked: boolean | 'indeterminate' = false;
  if (selectable) checked = entryCheckState(node, selected);

  if (isVbf && !isVbfContainerRoot) {
    const paths = vbfRoot ? (vbfSelectionByRoot.get(vbfRoot) ?? []) : [];
    const asSelectionNode = (child: SideNode): VbfSelectionTreeNode => ({
      path: child.vbfPath ?? child.entry?.vbf?.path ?? '',
      children: child.children?.map(asSelectionNode),
    });
    checked = vbfNodeCheckState(
      paths,
      vbfPath,
      node.children?.map(asSelectionNode)
    );
  }

  return (
    <div>
      <div
        style={{ paddingLeft: depth * 16 }}
        className="flex items-center pr-1 pb-1"
        data-node-id={node.id}
        data-node-kind={node.kind}
        data-node-label={node.label}
        data-node-entry={node.entry ? 'true' : undefined}
        data-node-vbf={isVbf ? 'true' : undefined}
        data-node-vbf-file={isVbf && !hasChildren && !isVbfContainerRoot ? 'true' : undefined}
        data-node-vbf-root={vbfRoot}
        data-node-vbf-path={vbfPath}
      >
        {node.loading ? (
          <span
            className="w-8 shrink-0 grid place-items-center opacity-60"
            aria-label={`Carregando ${node.label}`}
          >
            <Loader2 size={16} className="animate-spin" />
          </span>
        ) : hasChildren ? (
          <Button
            variant="ghost"
            size="icon"
            className="size-8 shrink-0"
            onClick={() => onToggle(node)}
            aria-label={`Alternar ${node.label}`}
          >
            {isExpanded ? <ChevronDown size={20} /> : <ChevronRight size={20} />}
          </Button>
        ) : (
          <span className="w-8 shrink-0" />
        )}
        {isVbfContainerRoot || isTextRoot ? null : (
          <Checkbox
            className="mr-1"
            checked={checked}
            onCheckedChange={(value) => onCheck(node, value === true)}
            aria-label={`Selecionar ${node.label}`}
          />
        )}
        <Button
          variant="ghost"
          size="sm"
          data-node-button
          className={`w-full justify-start text-left min-w-0 focus-visible:bg-primary/10 focus-visible:ring-1 focus-visible:ring-primary focus-visible:outline-none ${
            isSelected ? 'bg-primary/10 ring-1 ring-primary text-primary' : ''
          }`}
          onClick={() => onSelect(node)}
        >
          {/* Ícone do nó: diretório FECHADO quando colapsado e ABERTO quando
              expandido; arquivo de texto para binário com texto; imagem para
              .dds.phyre; arquivo genérico para formato fora do escopo. */}
          {node.entry ? (
            node.entry.kind === 'images' ? (
              <ImageIcon size={18} className="mr-2 shrink-0" />
            ) : (
              <FileText size={18} className="mr-2 shrink-0" />
            )
          ) : node.unsupported ? (
            <FileIcon size={18} className="mr-2 shrink-0 opacity-70" />
          ) : isExpanded ? (
            <FolderOpen size={18} className="mr-2 shrink-0" />
          ) : (
            <Folder size={18} className="mr-2 shrink-0" />
          )}
          <span className="min-w-0 flex-1 truncate">{node.label}</span>
        </Button>
      </div>
      {hasChildren && isExpanded
        ? (node.children ?? []).map((child) => (
            <TreeItem key={child.id} node={child} depth={depth + 1} />
          ))
        : null}
    </div>
  );
});

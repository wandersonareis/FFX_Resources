package services

import "sync"

// viewLoadMu serializa a INVALIDAÇÃO dos caches de view (lado ESCRITA do
// InvalidateViewCaches) contra as cargas bulk de store (lado LEITURA dos
// ensure*). O clear no MEIO de uma carga faria a carga repovoar o store
// com conteúdo lido da árvore ANTERIOR — o toggle EnableMods e a troca de
// gamefiles mudam a resolução mods-first, então a invalidação perderia o
// efeito e a UI serviria conteúdo velho "pós-recarga".
//
// Caso real (e2e mods-toggle-guard): o worker de pré-carga
// (PreloadVersions) aquecendo o Eternal Calm no exato instante do toggle;
// o store FFX recém-limpo era repovoado pelos eventos do preload em voo e
// o HasEvents(ffx) da listagem seguinte via "já carregado" — a árvore
// inteira virava as 2 folhas do preload.
//
// Leitores são PLANOS: nenhum ensure* chamado sob o RLock chama outro
// ensure* (o RWMutex do Go bloqueia novos leitores quando há escritor na
// fila — leitor aninhado deadlockaria consigo mesmo). O clear espera as
// cargas em voo terminarem; as cargas que vêm depois já veem o store
// limpo. A ordem de locks é sempre viewLoadMu → locks internos (readyMu,
// datastore, helpStore…), a mesma do escritor.
var viewLoadMu sync.RWMutex

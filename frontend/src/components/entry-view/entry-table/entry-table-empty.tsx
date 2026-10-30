export function EntryTableEmpty() {
  return (
    <div className="mt-2 rounded-md border border-dashed px-3 py-6 text-center text-sm text-muted-foreground">
      Nenhuma linha traduzível neste arquivo (todas em branco no original). As repetições de textos
      definidos em outro arquivo saem como link para a def — nada é ocultado.
    </div>
  );
}

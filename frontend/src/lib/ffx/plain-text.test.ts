import { describe, expect, it } from 'vitest';
import { formatReferencesForCopy, gameTextToPlainText } from './plain-text';

// A cópia das referências é o INVERSO do display: o texto que a área mostra
// como chips/cores/itálico precisa sair limpo e com as quebras preservadas.

describe('gameTextToPlainText', () => {
  it('remove tags de controle (cor, itálico e chips)', () => {
    expect(gameTextToPlainText('{CLR:RED}Oi{TEXT_NORMAL}, {TEXT_ITALIC}mundo{TEXT_NORMAL}!')).toBe(
      'Oi, mundo!',
    );
  });

  it('converte {TEXT_NEWLINE} e quebras reais em \\n', () => {
    expect(gameTextToPlainText('A{TEXT_NEWLINE}B\r\nC\nD\rE')).toBe('A\nB\nC\nD\nE');
  });

  it('reconhece {TEXT_NEWLINE} sem diferenciar maiúsculas', () => {
    expect(gameTextToPlainText('A{text_newline}B')).toBe('A\nB');
  });

  it('descarta o padding de tela do início (sem linhas em branco)', () => {
    expect(gameTextToPlainText('  \n{TEXT_NEWLINE}Oi')).toBe('Oi');
  });

  it('remove chips com valores entre chaves (CMD/HEX/VAR/MCR)', () => {
    expect(gameTextToPlainText('{MCR:s06:l3F:"Yevon"}Ola{VAR:01}{HEX:00112233}')).toBe('Ola');
  });

  it('mantém o espaçamento interno e o texto entre tags', () => {
    expect(gameTextToPlainText('{CLR:BLUE}  recuado  {TEXT_NORMAL}')).toBe('  recuado  ');
  });

  it('texto vazio ou ausente devolve vazio', () => {
    expect(gameTextToPlainText(undefined)).toBe('');
    expect(gameTextToPlainText('')).toBe('');
  });
});

describe('formatReferencesForCopy', () => {
  it('junta todas as línguas com o cabeçalho exibido e linha em branco', () => {
    const text = formatReferencesForCopy([
      { name: 'Alemão', code: 'de', text: '{TEXT_ITALIC}Hallo{TEXT_NORMAL}' },
      { name: 'Japonês', code: 'jp', text: 'こんにちは{TEXT_NEWLINE}世界' },
    ]);
    expect(text).toBe('Alemão (de)\nHallo\n\nJaponês (jp)\nこんにちは\n世界');
  });

  it('pula referências sem texto legível (vazias ou só com tags)', () => {
    const text = formatReferencesForCopy([
      { name: 'Alemão', code: 'de', text: '' },
      { name: 'Francês', code: 'fr', text: '{CLR:RED}{TEXT_NORMAL}' },
      { name: 'Italiano', code: 'it', text: 'Ciao' },
    ]);
    expect(text).toBe('Italiano (it)\nCiao');
  });

  it('sem referências copiáveis devolve string vazia', () => {
    expect(formatReferencesForCopy([])).toBe('');
    expect(formatReferencesForCopy([{ name: 'Alemão', code: 'de', text: '  ' }])).toBe('');
  });
});

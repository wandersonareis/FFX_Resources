import { describe, expect, it } from 'vitest';
import { formatEventGroupLabel, resolveEventGroup, shortenedOf } from './event-group-names';

describe('shortenedOf', () => {
  it('corta o eventID em 4 caracteres (o fragmento é a chave)', () => {
    expect(shortenedOf('azit0000')).toBe('azit');
    expect(shortenedOf('Azit0000')).toBe('azit');
    expect(shortenedOf('dream0000')).toBe('drea');
    expect(shortenedOf('mmmc0000')).toBe('mmmc');
  });
});

describe('resolveEventGroup', () => {
  it('aplica o nome real por fragmento exato em todas as versões', () => {
    expect(resolveEventGroup('ffx', 'bika', ['bika']).name).toBe('Bikanel Island');
    expect(resolveEventGroup('ffx2', 'bika', ['bika']).name).toBe('Bikanel Island');
    expect(resolveEventGroup('lastmiss', 'bika', ['bika']).name).toBe('Bikanel Island');
    expect(resolveEventGroup('eternalcalm', 'azit', ['azit']).name).toBe('Home');
    expect(resolveEventGroup('ffx', 'mihn', ['mihn']).name).toBe("Mi'hen Highroad");
  });

  it('não casa por prefixo: o fragmento tem que ser exato', () => {
    expect(resolveEventGroup('ffx', 'bik', ['bik']).name).toBe('');
    expect(resolveEventGroup('ffx', 'azit9', ['azit9']).name).toBe('');
  });

  it('dream#### tem a chave drea, não dream', () => {
    expect(resolveEventGroup('ffx', 'drea', ['drea']).name).toBe('Dream Zanarkand');
    expect(resolveEventGroup('ffx', 'dream', ['dream']).name).toBe('');
  });

  it('mmmc mostra só o diretório pai mm, sem local inventado', () => {
    expect(resolveEventGroup('ffx', 'mmmc', ['mmmc'])).toEqual({
      target: 'mmmc',
      name: 'mm',
    });
  });

  it('continua fundindo fragmento sem nome no primeiro nomeado do shortened', () => {
    expect(resolveEventGroup('ffx', 'azmm', ['azit', 'azmm'])).toEqual({
      target: 'azit',
      name: 'Home',
    });
    expect(resolveEventGroup('ffx', 'bsyt', ['bsil', 'bsvr', 'bsyt'])).toEqual({
      target: 'bsil',
      name: 'Besaid Island',
    });
  });

  // Regressão: o caller de produção (eventGroups) passa TODOS os fragmentos,
  // sem pré-filtrar por shortened. Sem o filtro, o sort() escolhia "azit" —
  // o menor fragmento nomeado da árvore inteira — e todo órfão de qualquer
  // pasta caía em Home.
  it('não mistura shortenings quando o caller passa todos os fragmentos', () => {
    const todos = [
      'azit',
      'azmm',
      'bsil',
      'bsvr',
      'bsyt',
      'bvyt',
      'genk',
      'gemm',
      'kino',
      'lchb',
      'lumm',
      'mihn',
      'maca',
      'mamm',
      'isho',
      'kamm',
    ];
    // bsyt não pode pular para "azit" (Home) só porque ele é o menor nomeado.
    expect(resolveEventGroup('ffx', 'bsyt', todos)).toEqual({
      target: 'bsil',
      name: 'Besaid Island',
    });
    // mm/sufixo também fica no próprio shortened.
    expect(resolveEventGroup('ffx', 'mamm', todos)).toEqual({
      target: 'maca',
      name: 'Lake Macalania',
    });
    // Azmm é do MESMO shortened de azit → continua em Home.
    expect(resolveEventGroup('ffx', 'azmm', todos)).toEqual({
      target: 'azit',
      name: 'Home',
    });
    // Sem irmão nomeado no shortened, vira nó próprio (pasta no disco).
    expect(resolveEventGroup('ffx', 'isho', todos)).toEqual({
      target: 'isho',
      name: '',
    });
    expect(resolveEventGroup('ffx', 'kamm', todos)).toEqual({
      target: 'kamm',
      name: '',
    });
  });

  it('scen só é nomeado na aba eternalcalm', () => {
    expect(resolveEventGroup('eternalcalm', 'scen', ['scen']).name).toBe('Eternal Calm');
    expect(resolveEventGroup('ffx', 'scen', ['scen']).name).toBe('');
  });
});

describe('formatEventGroupLabel', () => {
  it('sufixa o diretório pai no nome do local', () => {
    expect(formatEventGroupLabel('Home', 'azit', 12)).toBe('Home (az) - 12');
    expect(formatEventGroupLabel('Besaid Temple', 'bvyt', 9)).toBe('Besaid Temple (bv) - 9');
  });

  it('omite o sufixo quando o nome já é o código do pai', () => {
    expect(formatEventGroupLabel('mm', 'mmmc', 5)).toBe('mm - 5');
  });

  it('sem nome, exibe o fragmento cru igual à pasta no disco', () => {
    expect(formatEventGroupLabel('', 'azmm', 3)).toBe('azmm - 3');
  });
});

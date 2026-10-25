import { describe, expect, it } from 'vitest';
import { GAME_VERSIONS } from './game-version';
import { SHORTCUTS } from './shortcuts';

// Uma tecla de aba por versão: o registro (app-shell) e o tooltip por aba
// derivam do MESMO array, então este é o único acoplamento que precisa bater.
describe('shortcuts', () => {
  it('tem uma tecla de aba por versão de GAME_VERSIONS', () => {
    expect(SHORTCUTS.versionTabs.keys).toHaveLength(GAME_VERSIONS.length);
  });
});

import { buildCriteria, runQueryAll } from './query';
import type { SmartZoneClient } from './client';

describe('buildCriteria', () => {
  it('pages from one, as SmartZone expects', () => {
    expect(buildCriteria()).toMatchObject({ page: 1, limit: 50 });
  });

  it('omits empty filter arrays', () => {
    // An empty `filters: []` is read as "match nothing" by some builds, so it
    // must not be sent at all.
    const criteria = buildCriteria({ filters: [], extraFilters: [undefined, false] });
    expect(criteria).not.toHaveProperty('filters');
    expect(criteria).not.toHaveProperty('extraFilters');
  });

  it('drops falsy filters but keeps the real ones', () => {
    const criteria = buildCriteria({
      filters: [{ type: 'ZONE', value: 'z1' }, undefined, false],
    });
    expect(criteria.filters).toEqual([{ type: 'ZONE', value: 'z1' }]);
  });

  it('ignores a blank search box', () => {
    expect(buildCriteria({ search: '   ' })).not.toHaveProperty('fullTextSearch');
    expect(buildCriteria({ search: 'lobby' }).fullTextSearch).toEqual({
      type: 'AND',
      value: 'lobby',
    });
  });

  it('passes sorting through as given', () => {
    expect(buildCriteria({ sort: { sortColumn: 'rssi', dir: 'ASC' } }).sortInfo).toEqual({
      sortColumn: 'rssi',
      dir: 'ASC',
    });
  });
});

describe('runQueryAll', () => {
  it('pages until hasMore goes false', async () => {
    const pages = [
      { totalCount: 3, hasMore: true, firstIndex: 0, list: ['a'] },
      { totalCount: 3, hasMore: true, firstIndex: 1, list: ['b'] },
      { totalCount: 3, hasMore: false, firstIndex: 2, list: ['c'] },
    ];
    let i = 0;
    const client = { post: jest.fn(async () => pages[i++]) } as unknown as SmartZoneClient;

    await expect(runQueryAll<string>(client, '/query/ap', {})).resolves.toEqual([
      'a',
      'b',
      'c',
    ]);
  });

  it('stops on an empty page even if the controller still claims more', async () => {
    const client = {
      post: jest.fn(async () => ({ totalCount: 9, hasMore: true, firstIndex: 0, list: [] })),
    } as unknown as SmartZoneClient;

    await expect(runQueryAll(client, '/query/ap', {})).resolves.toEqual([]);
    expect(client.post).toHaveBeenCalledTimes(1);
  });
});

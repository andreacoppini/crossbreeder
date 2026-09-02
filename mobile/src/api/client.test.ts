import {
  SmartZoneClient,
  buildUrl,
  compareApiVersions,
  originFor,
  pickApiVersion,
  withPath,
} from './client';
import { SmartZoneError } from './errors';

/** A fetch stand-in that records what it was asked and answers from a script. */
function mockFetch(
  handler: (url: string, init: RequestInit) => {
    status?: number;
    body?: unknown;
    throws?: unknown;
  },
) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fn = jest.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    const res = handler(url, init);
    if (res.throws) throw res.throws;
    const status = res.status ?? 200;
    return {
      ok: status >= 200 && status < 300,
      status,
      headers: new Headers(),
      text: async () => (res.body === undefined ? '' : JSON.stringify(res.body)),
    } as unknown as Response;
  });
  globalThis.fetch = fn as unknown as typeof fetch;
  return { calls };
}

const endpoint = { host: 'sz.example.com', port: 8443 };
const credentials = { username: 'admin', password: 'secret' };

afterEach(() => {
  jest.restoreAllMocks();
});

describe('pickApiVersion', () => {
  it('takes the newest version both sides know', () => {
    expect(pickApiVersion(['v9_0', 'v10_0', 'v11_0'])).toBe('v11_0');
  });

  it('compares numerically, not as strings', () => {
    // 'v9_0' sorts after 'v11_0' lexically, which is the bug this guards.
    expect(pickApiVersion(['v11_0', 'v9_0'])).toBe('v11_0');
    expect(compareApiVersions('v9_0', 'v11_0')).toBeLessThan(0);
  });

  it('falls back when the controller offers nothing we know', () => {
    expect(pickApiVersion(['v99_9'])).toBe('v99_9');
    expect(pickApiVersion([])).toBe('v11_0');
    expect(pickApiVersion(undefined)).toBe('v11_0');
  });
});

describe('originFor', () => {
  it('builds an https origin with the port', () => {
    expect(originFor({ host: 'sz.example.com' })).toBe('https://sz.example.com:8443');
    expect(originFor({ host: '10.1.1.5', port: 443 })).toBe('https://10.1.1.5:443');
  });

  it('brackets an IPv6 literal so the port is not misread', () => {
    expect(originFor({ host: 'fd00::1' })).toBe('https://[fd00::1]:8443');
  });
});

describe('buildUrl', () => {
  it('appends the ticket and escapes it', () => {
    const url = buildUrl('https://h:8443/wsg/api/public/v11_0', '/rkszones', undefined, 'a b+c');
    expect(url).toBe(
      'https://h:8443/wsg/api/public/v11_0/rkszones?serviceTicket=a%20b%2Bc',
    );
  });

  it('drops undefined query values rather than sending "undefined"', () => {
    const url = buildUrl('https://h', '/x', { a: 1, b: undefined, c: null }, undefined);
    expect(url).toBe('https://h/x?a=1');
  });

  it('keeps a path that already has a query intact', () => {
    const url = buildUrl('https://h', '/x?y=1', undefined, 't');
    expect(url).toBe('https://h/x?y=1&serviceTicket=t');
  });
});

describe('withPath', () => {
  it('escapes values so a MAC with a colon is safe', () => {
    expect(withPath('/aps/{apMac}/reboot', { apMac: 'AA:BB:CC' })).toBe(
      '/aps/AA%3ABB%3ACC/reboot',
    );
  });

  it('refuses to build a URL with a hole in it', () => {
    expect(() => withPath('/aps/{apMac}', {})).toThrow(/Missing path parameter/);
  });
});

describe('SmartZoneClient session handling', () => {
  it('negotiates a version, logs in, and carries the ticket', async () => {
    const { calls } = mockFetch((url) => {
      if (url.includes('/apiInfo')) {
        return { body: { apiSupportVersions: ['v9_0', 'v11_0'] } };
      }
      if (url.includes('/serviceTicket')) {
        return { body: { serviceTicket: 'TICKET-1', controllerVersion: '6.1.2' } };
      }
      return { body: { totalCount: 0, list: [] } };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await client.get('/rkszones');

    expect(calls[1]?.url).toContain('/wsg/api/public/v11_0/serviceTicket');
    expect(calls[2]?.url).toContain('serviceTicket=TICKET-1');
    expect(client.currentSession?.controllerVersion).toBe('6.1.2');
  });

  it('re-logs in once and replays the call when the ticket has expired', async () => {
    let ticket = 0;
    let firstCallRejected = false;

    const { calls } = mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        ticket += 1;
        return { body: { serviceTicket: `T${ticket}`, controllerVersion: '6.1' } };
      }
      if (!firstCallRejected) {
        firstCallRejected = true;
        return { status: 401, body: { message: 'Ticket expired' } };
      }
      return { body: { ok: true } };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    const result = await client.get<{ ok: boolean }>('/rkszones');

    expect(result).toEqual({ ok: true });
    expect(ticket).toBe(2);
    // The replay carries the new ticket, not the dead one.
    expect(calls[calls.length - 1]?.url).toContain('serviceTicket=T2');
  });

  it('does not stampede the controller when many calls hit a cold session', async () => {
    let logins = 0;
    mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        logins += 1;
        return { body: { serviceTicket: 'T', controllerVersion: '6.1' } };
      }
      return { body: {} };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await Promise.all([
      client.get('/a'),
      client.get('/b'),
      client.get('/c'),
      client.get('/d'),
    ]);

    expect(logins).toBe(1);
  });

  it('gives up rather than looping when the replay is refused too', async () => {
    mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        return { body: { serviceTicket: 'T', controllerVersion: '6.1' } };
      }
      return { status: 401, body: { message: 'Nope' } };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await expect(client.get('/rkszones')).rejects.toMatchObject({
      kind: 'auth',
      status: 401,
    });
  });

  it('reuses a ticket recovered from storage instead of logging in', async () => {
    let logins = 0;
    mockFetch((url) => {
      if (url.includes('/serviceTicket')) {
        logins += 1;
        return { body: { serviceTicket: 'NEW', controllerVersion: '6.1' } };
      }
      return { body: { ok: true } };
    });

    const client = new SmartZoneClient({
      endpoint,
      credentials,
      initialSession: {
        serviceTicket: 'CACHED',
        controllerVersion: '6.1',
        apiVersion: 'v11_0',
        issuedAt: Date.now(),
      },
    });
    await client.get('/rkszones');
    expect(logins).toBe(0);
  });

  it('surfaces the controller message on a rejected change', async () => {
    mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        return { body: { serviceTicket: 'T', controllerVersion: '6.1' } };
      }
      return {
        status: 422,
        body: { message: 'VLAN 5000 is out of range' },
      };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await expect(client.patch('/x', {})).rejects.toMatchObject({
      kind: 'conflict',
      fault: { message: 'VLAN 5000 is out of range' },
    });
  });
});

describe('listAll', () => {
  it('walks pages until the controller says there are no more', async () => {
    const pages = [
      { totalCount: 5, hasMore: true, firstIndex: 0, list: [1, 2] },
      { totalCount: 5, hasMore: true, firstIndex: 2, list: [3, 4] },
      { totalCount: 5, hasMore: false, firstIndex: 4, list: [5] },
    ];
    let page = 0;
    mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        return { body: { serviceTicket: 'T', controllerVersion: '6.1' } };
      }
      return { body: pages[page++] };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await expect(client.listAll<number>('/rkszones')).resolves.toEqual([1, 2, 3, 4, 5]);
  });

  it('stops at the page cap rather than pulling an unbounded set', async () => {
    let requests = 0;
    mockFetch((url) => {
      if (url.includes('/apiInfo')) return { body: { apiSupportVersions: ['v11_0'] } };
      if (url.includes('/serviceTicket')) {
        return { body: { serviceTicket: 'T', controllerVersion: '6.1' } };
      }
      requests += 1;
      return { body: { totalCount: 1e9, hasMore: true, firstIndex: 0, list: [1] } };
    });

    const client = new SmartZoneClient({ endpoint, credentials });
    await client.listAll('/rkszones', { maxPages: 3 });
    expect(requests).toBe(3);
  });
});

describe('transport failures', () => {
  it('reads a rejected certificate as its own kind, not a dead host', async () => {
    mockFetch(() => ({
      throws: Object.assign(new TypeError('SSLHandshake: certificate verify failed'), {
        name: 'TypeError',
      }),
    }));

    const client = new SmartZoneClient({ endpoint, credentials });
    await expect(client.apiInfo()).rejects.toMatchObject({ kind: 'tls' });
  });

  it('reads an unreachable host as a network failure', async () => {
    mockFetch(() => ({ throws: new TypeError('Network request failed') }));
    const client = new SmartZoneClient({ endpoint, credentials });
    await expect(client.apiInfo()).rejects.toMatchObject({ kind: 'network' });
  });

  it('marks network and server errors retryable, and refusals not', () => {
    expect(new SmartZoneError('network', 'x').retryable).toBe(true);
    expect(new SmartZoneError('server', 'x').retryable).toBe(true);
    expect(new SmartZoneError('forbidden', 'x').retryable).toBe(false);
    expect(new SmartZoneError('conflict', 'x').retryable).toBe(false);
  });
});

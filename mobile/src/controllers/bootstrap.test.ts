import { parseBootstrap, toBootstrapLink } from './bootstrap';

describe('parseBootstrap', () => {
  it('reads the app’s own deep link', () => {
    expect(
      parseBootstrap('szconsole://connect?host=sz.example.com&port=8443&user=admin&label=HQ'),
    ).toEqual({
      host: 'sz.example.com',
      port: 8443,
      username: 'admin',
      label: 'HQ',
    });
  });

  it('reads a JSON payload', () => {
    expect(
      parseBootstrap('{"host":"10.1.20.5","port":9443,"username":"noc","label":"Site B"}'),
    ).toEqual({ host: '10.1.20.5', port: 9443, username: 'noc', label: 'Site B' });
  });

  it('reads a URL pasted out of a browser, path and all', () => {
    expect(parseBootstrap('https://sz.example.com:8443/wsg/api/public/v11_0')).toEqual({
      host: 'sz.example.com',
      port: 8443,
    });
  });

  it('reads a bare host, and a host with a port', () => {
    expect(parseBootstrap('sz.example.com')).toEqual({ host: 'sz.example.com' });
    expect(parseBootstrap('10.1.20.5:9443')).toEqual({ host: '10.1.20.5', port: 9443 });
  });

  it('does not mistake an IPv6 literal’s colons for a port', () => {
    expect(parseBootstrap('fd00::1')).toEqual({ host: 'fd00::1' });
    expect(parseBootstrap('[fd00::1]:8443')).toEqual({ host: 'fd00::1', port: 8443 });
  });

  it('drops a path or query glued onto a bare host', () => {
    expect(parseBootstrap('sz.example.com/wsg')).toEqual({ host: 'sz.example.com' });
  });

  it('trims what was typed', () => {
    expect(parseBootstrap('   sz.example.com  ')).toEqual({ host: 'sz.example.com' });
  });

  it('rejects an empty or nonsense input rather than guessing', () => {
    expect(parseBootstrap('')).toBeNull();
    expect(parseBootstrap('   ')).toBeNull();
    expect(parseBootstrap('not a host!')).toBeNull();
    expect(parseBootstrap('{"nope":true}')).toBeNull();
    expect(parseBootstrap('{broken json')).toBeNull();
  });

  it('ignores a port that could not be one', () => {
    expect(parseBootstrap('sz.example.com:99999')).toEqual({ host: 'sz.example.com' });
  });

  it('never carries a password, whatever the payload claims', () => {
    const parsed = parseBootstrap(
      'szconsole://connect?host=sz.example.com&user=admin&password=hunter2',
    );
    expect(parsed).not.toBeNull();
    expect(JSON.stringify(parsed)).not.toContain('hunter2');
    expect(parsed && 'password' in parsed).toBe(false);
  });
});

describe('toBootstrapLink', () => {
  it('round-trips through the parser', () => {
    const payload = {
      host: 'sz.example.com',
      port: 9443,
      username: 'admin',
      label: 'HQ',
    };
    expect(parseBootstrap(toBootstrapLink(payload))).toEqual(payload);
  });

  it('leaves the default port out of the link', () => {
    expect(toBootstrapLink({ host: 'sz.example.com', port: 8443 })).not.toContain('port=');
  });

  it('never puts a secret in the link', () => {
    const link = toBootstrapLink({ host: 'h', username: 'admin', label: 'HQ' });
    expect(link).not.toMatch(/pass|secret|ticket/i);
  });
});

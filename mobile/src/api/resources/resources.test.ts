/**
 * Rules learned from a live SmartZone 7.1.1 cluster.
 *
 * Every case here corresponds to something the controller does that the
 * published schema does not make obvious, and most of them correspond to a
 * bug this app had before it was pointed at real hardware.
 */

import {
  bandForClient,
  isAuthorised,
  sessionDuration,
  signalReason,
  signalVerdict,
} from './clients';
import { dpskToCsv, expiryDate, filterByWlan } from './dpsk';
import { alarmTotal, isAcknowledged } from './alarms';
import { apCapacityUsed } from './system';
import {
  accessVlan,
  isDpskWlan,
  isExternalDpskWlan,
  isOpenWlan,
  isSsidBroadcast,
  wlanPatch,
  type Wlan,
} from './wlans';

describe('signal verdict', () => {
  it('treats a zero reading as no reading, not as a perfect one', () => {
    // The controller sends rssi/snr of 0 for a client it has not measured
    // yet. Scoring that as a number reports a dead client as excellent.
    expect(signalVerdict({ rssi: 0, snr: 0 })).toBe('unknown');
    expect(signalReason({ rssi: 0, snr: 0 })).toMatch(/no signal reading/i);
  });

  it('grades real readings', () => {
    expect(signalVerdict({ rssi: -55, snr: 35 })).toBe('good');
    expect(signalVerdict({ rssi: -70, snr: 30 })).toBe('fair');
    expect(signalVerdict({ rssi: -80, snr: 30 })).toBe('poor');
  });

  it('lets a bad SNR fail a client whose RSSI looks fine', () => {
    expect(signalVerdict({ rssi: -50, snr: 10 })).toBe('poor');
  });

  it('says unknown when nothing was reported at all', () => {
    expect(signalVerdict({})).toBe('unknown');
  });
});

describe('authorisation', () => {
  it('reads the controller’s uppercase spelling', () => {
    expect(isAuthorised({ authStatus: 'AUTHORIZED' })).toBe(true);
    expect(isAuthorised({ authStatus: 'UNAUTHORIZED' })).toBe(false);
  });

  it('does not cry wolf when nothing was reported', () => {
    expect(isAuthorised({})).toBe(true);
  });
});

describe('client band', () => {
  it('comes from the channel, since radioType is a PHY string', () => {
    expect(bandForClient({ channel: 6, radioType: 'n/ax' })).toBe('2.4 GHz');
    expect(bandForClient({ channel: 116, radioType: 'a/n/ac/ax/be' })).toBe('5 GHz');
    expect(bandForClient({ channel: 197, radioType: 'a/n/ac/ax/be' })).toBe('6 GHz');
  });

  it('says nothing rather than guessing when there is no channel', () => {
    expect(bandForClient({})).toBeNull();
    expect(bandForClient({ channel: 0 })).toBeNull();
  });
});

describe('session duration', () => {
  it('is computed, because the controller sends no duration field', () => {
    const tenMinutesAgo = Date.now() - 600_000;
    const seconds = sessionDuration({ sessionStartTime: tenMinutesAgo });
    expect(seconds).toBeGreaterThanOrEqual(599);
    expect(seconds).toBeLessThanOrEqual(601);
  });

  it('handles a second-precision timestamp', () => {
    const seconds = sessionDuration({
      sessionStartTime: Math.floor((Date.now() - 60_000) / 1000),
    });
    expect(seconds).toBeGreaterThanOrEqual(59);
  });

  it('is undefined when the session never started', () => {
    expect(sessionDuration({})).toBeUndefined();
    expect(sessionDuration({ sessionStartTime: 0 })).toBeUndefined();
  });
});

describe('alarms', () => {
  it('reads acknowledged as the string it is, not as truthiness', () => {
    // "No" is a truthy string. Reading it as a boolean marks every open
    // alarm acknowledged, which is exactly backwards.
    expect(isAcknowledged({ acknowledged: 'No' })).toBe(false);
    expect(isAcknowledged({ acknowledged: 'Yes' })).toBe(true);
    expect(isAcknowledged({})).toBe(false);
  });

  it('adds up a summary that carries no total', () => {
    expect(
      alarmTotal({ criticalCount: 1, majorCount: 2, minorCount: 3, warningCount: 4 }),
    ).toBe(10);
    expect(alarmTotal(undefined)).toBeUndefined();
  });
});

describe('DPSK', () => {
  it('narrows to a WLAN locally, because the API filter matches nothing', () => {
    const keys = [
      { key: 'a', wlanId: '4' },
      { key: 'b', wlanId: '23' },
      { key: 'c', wlanId: '4' },
    ];
    expect(filterByWlan(keys, '4').map((k) => k.key)).toEqual(['a', 'c']);
    expect(filterByWlan(keys, undefined)).toHaveLength(3);
  });

  it('compares wlanId as a string, since the API mixes the two', () => {
    expect(filterByWlan([{ key: 'a', wlanId: '4' }], '4')).toHaveLength(1);
  });

  it('reads zero expiry as never', () => {
    expect(expiryDate({ expirationTime: 0 })).toBeNull();
    expect(expiryDate({})).toBeNull();
    expect(expiryDate({ expirationTime: 1_700_000_000_000 })).toBeInstanceOf(Date);
  });

  it('exports no passphrase column, because there is no passphrase to give', () => {
    const csv = dpskToCsv([
      { key: 'k1', userName: 'Flat 3B', vlanId: 24, wlanId: '4', group: true },
    ]);
    expect(csv).not.toMatch(/passphrase/i);
    expect(csv.split('\n')[0]).toContain('User name');
    expect(csv).toContain('Flat 3B');
  });

  it('quotes a name containing a comma', () => {
    const csv = dpskToCsv([{ key: 'k', userName: 'Smith, John' }]);
    expect(csv).toContain('"Smith, John"');
  });
});

describe('WLAN shape', () => {
  const wlan: Wlan = {
    id: '23',
    name: 'HRH-Staff',
    ssid: 'HRH-Staff',
    vlan: { accessVlan: 4, aaaVlanOverride: true },
    encryption: { method: 'WPA2', algorithm: 'AES', passphrase: 'secret-key' },
    dpsk: { dpskEnabled: true, length: 10 },
    externalDpsk: { enabled: false },
    advancedOptions: { hideSsidEnabled: false, clientIsolationEnabled: true },
  };

  it('reads the nested fields the controller actually uses', () => {
    expect(accessVlan(wlan)).toBe(4);
    expect(isDpskWlan(wlan)).toBe(true);
    expect(isExternalDpskWlan(wlan)).toBe(false);
    expect(isOpenWlan(wlan)).toBe(false);
  });

  it('inverts hideSsidEnabled into the concept the UI shows', () => {
    expect(isSsidBroadcast(wlan)).toBe(true);
    expect(isSsidBroadcast({ advancedOptions: { hideSsidEnabled: true } })).toBe(false);
    // Absent means broadcasting, which is SmartZone's default.
    expect(isSsidBroadcast({})).toBe(true);
  });

  it('treats a WLAN with no encryption as open', () => {
    expect(isOpenWlan({ encryption: { method: 'None' } })).toBe(true);
    expect(isOpenWlan({})).toBe(true);
  });
});

describe('wlanPatch', () => {
  const wlan: Wlan = {
    name: 'Staff',
    ssid: 'Staff',
    vlan: { accessVlan: 4, aaaVlanOverride: true, coreSVlan: null },
    encryption: { method: 'WPA2', algorithm: 'AES', passphrase: 'old' },
    advancedOptions: { hideSsidEnabled: false, clientIsolationEnabled: true },
  };

  it('sends nothing when nothing changed', () => {
    expect(
      wlanPatch(wlan, { name: 'Staff', ssid: 'Staff', broadcast: true }),
    ).toEqual({});
  });

  it('sends only what moved', () => {
    expect(wlanPatch(wlan, { name: 'Staff Wi-Fi' })).toEqual({ name: 'Staff Wi-Fi' });
  });

  it('keeps a nested object whole when changing one key inside it', () => {
    // Sending {vlan: {accessVlan: 9}} alone drops aaaVlanOverride, which the
    // controller reads as clearing it.
    const patch = wlanPatch(wlan, { accessVlan: 9 });
    expect(patch.vlan).toEqual({ accessVlan: 9, aaaVlanOverride: true, coreSVlan: null });
  });

  it('carries the rest of advancedOptions when toggling broadcast', () => {
    const patch = wlanPatch(wlan, { broadcast: false });
    expect(patch.advancedOptions).toEqual({
      hideSsidEnabled: true,
      clientIsolationEnabled: true,
    });
  });

  it('carries the encryption settings alongside a new passphrase', () => {
    const patch = wlanPatch(wlan, { passphrase: 'a-new-one' });
    expect(patch.encryption).toEqual({
      method: 'WPA2',
      algorithm: 'AES',
      passphrase: 'a-new-one',
    });
  });
});

describe('devicesSummary', () => {
  it('works out licensed capacity used', () => {
    // Real figures from the cluster this was verified against.
    expect(
      apCapacityUsed({ totalApCapacity: 2785, totalRemainingApCapacity: 2198 }),
    ).toBe(21);
  });

  it('declines to divide by nothing', () => {
    expect(apCapacityUsed({})).toBeUndefined();
    expect(apCapacityUsed({ totalApCapacity: 0, totalRemainingApCapacity: 0 })).toBeUndefined();
  });
});

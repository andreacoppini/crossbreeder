import {
  formatBytes,
  formatDuration,
  formatMac,
  formatRelative,
  normaliseEpoch,
  normaliseMac,
} from './format';

describe('formatBytes', () => {
  it('keeps small values in bytes', () => {
    expect(formatBytes(512)).toBe('512 B');
  });

  it('steps up through the units', () => {
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB');
  });

  it('drops the decimal once the number is wide', () => {
    expect(formatBytes(200 * 1024 * 1024)).toBe('200 MB');
  });

  it('answers for missing values rather than throwing', () => {
    expect(formatBytes(undefined)).toBe('—');
    expect(formatBytes(Number.NaN)).toBe('—');
  });
});

describe('formatDuration', () => {
  it('shows at most two units', () => {
    expect(formatDuration(4 * 86400 + 6 * 3600 + 725)).toBe('4d 6h');
    expect(formatDuration(3 * 3600 + 20 * 60)).toBe('3h 20m');
    expect(formatDuration(90)).toBe('1m');
    expect(formatDuration(30)).toBe('30s');
  });

  it('omits a zero second unit', () => {
    expect(formatDuration(2 * 86400)).toBe('2d');
  });
});

describe('normaliseEpoch', () => {
  it('promotes second-precision timestamps to milliseconds', () => {
    expect(normaliseEpoch(1_700_000_000)).toBe(1_700_000_000_000);
  });

  it('leaves millisecond timestamps alone', () => {
    expect(normaliseEpoch(1_700_000_000_000)).toBe(1_700_000_000_000);
  });

  it('rejects zero and negatives', () => {
    expect(normaliseEpoch(0)).toBeNull();
    expect(normaliseEpoch(-5)).toBeNull();
  });
});

describe('formatRelative', () => {
  it('reads recent times as just now', () => {
    expect(formatRelative(Date.now() - 1000)).toBe('just now');
  });

  it('counts minutes', () => {
    expect(formatRelative(Date.now() - 12 * 60_000)).toBe('12 min ago');
  });
});

describe('MAC handling', () => {
  it('normalises whatever separator arrived', () => {
    expect(formatMac('aabbccddeeff')).toBe('AA:BB:CC:DD:EE:FF');
    expect(formatMac('AA-BB-CC-DD-EE-FF')).toBe('AA:BB:CC:DD:EE:FF');
  });

  it('leaves a value it cannot parse recognisable', () => {
    expect(formatMac('not-a-mac')).toBe('NOT-A-MAC');
  });

  it('strips to bare hex for comparison', () => {
    expect(normaliseMac('AA:BB:CC:DD:EE:FF')).toBe('aabbccddeeff');
  });
});

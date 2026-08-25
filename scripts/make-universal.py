#!/usr/bin/env python3
"""Join two Mach-O binaries into one universal ("fat") binary.

Apple's own tool for this is lipo, which only exists on macOS. The container
format is a small big-endian header, so building it here keeps the whole
release on one runner.

    make-universal.py <amd64> <arm64> <out>
"""
import struct
import sys

CPU_TYPE = {"amd64": (0x01000007, 3), "arm64": (0x0100000C, 0)}
ALIGN = 14  # 2^14 = 16 KiB; arm64 slices must start on that boundary


def main(amd64_path: str, arm64_path: str, out_path: str) -> None:
    slices = [
        (CPU_TYPE["amd64"], open(amd64_path, "rb").read()),
        (CPU_TYPE["arm64"], open(arm64_path, "rb").read()),
    ]
    for _, body in slices:
        if body[:4] not in (b"\xcf\xfa\xed\xfe", b"\xce\xfa\xed\xfe"):
            raise SystemExit("not a Mach-O binary: wrong input?")

    step = 1 << ALIGN
    pos = 8 + 20 * len(slices)
    offsets = []
    for _, body in slices:
        pos = (pos + step - 1) & ~(step - 1)
        offsets.append(pos)
        pos += len(body)

    fat = struct.pack(">II", 0xCAFEBABE, len(slices))
    for ((cpu, sub), body), off in zip(slices, offsets):
        fat += struct.pack(">IIIII", cpu, sub, off, len(body), ALIGN)
    for (_, body), off in zip(slices, offsets):
        fat += b"\0" * (off - len(fat)) + body

    with open(out_path, "wb") as f:
        f.write(fat)
    print(f"{out_path}: {len(fat)} bytes, {len(slices)} architectures")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit(__doc__)
    main(*sys.argv[1:])

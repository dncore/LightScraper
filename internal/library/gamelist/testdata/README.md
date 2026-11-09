# gamelist.xml fixtures

These fixtures are test input only. They contain no ROMs, images, videos, or
other media files.

## Official Batocera 43.1 samples

Source repository: <https://github.com/batocera-linux/batocera.linux>

- Tag: `batocera-43.1`
- Commit: `155c2d8d304cbb53db52e9479dcf683392821d5c`
- Repository license: GPL-2.0
  ([`COPYING`](https://github.com/batocera-linux/batocera.linux/blob/batocera-43.1/COPYING))

### `batocera-43.1-nes-gamelist.xml`

- Original path:
  `package/batocera/emulationstation/batocera-es-system/roms/nes/gamelist.xml`
- Original URL:
  <https://github.com/batocera-linux/batocera.linux/blob/batocera-43.1/package/batocera/emulationstation/batocera-es-system/roms/nes/gamelist.xml>
- Bytes: 859
- SHA-256:
  `bd0fb1afbfe0f726a589cbbb80da1b8465f3f04771db72991ceb0a37c5ef9c0b`
- Starts with UTF-8 BOM (`EF BB BF`).
- Unmodified official file. It includes one game, a `./` ROM path, a long
  single-line description, `cheevosHash`, `cheevosId`, and play statistics.

### `batocera-43.1-c64-gamelist.xml`

- Original path:
  `package/batocera/emulationstation/batocera-es-system/roms/c64/gamelist.xml`
- Original URL:
  <https://github.com/batocera-linux/batocera.linux/blob/batocera-43.1/package/batocera/emulationstation/batocera-es-system/roms/c64/gamelist.xml>
- Bytes: 2754
- SHA-256:
  `4254b0ee3c797edb22cbe3a08d1b6422040d5362fe4bd09ed9c684626f4dd2df`
- Starts with UTF-8 BOM (`EF BB BF`).
- Unmodified official file. It includes a multiline `<desc>` with raw line
  breaks and apostrophes.

The upstream EmulationStation metadata implementation is MIT-licensed. The
Batocera Linux repository that distributes these gamelist samples is
GPL-2.0. The provenance above is retained so the fixture origin and license are
auditable. The fixture descriptions are game metadata, not ROM content.

## Synthetic fixture

### `synthetic-complex-gamelist.xml`

- **Synthetic**, not an upstream sample.
- Purpose: cover cases absent from the official samples: default and prefixed
  namespaces, XML comments, CDATA, unknown nested elements, unknown attributes,
  duplicate `<name>` and `<desc>` fields, empty and self-closing elements,
  `parentHash`, `<scrap>`, a folder, and an unknown top-level element.
- It is intentionally valid XML and is used to prove that parse/marshal
  preserves every byte when no edit is made, and only changes the target span
  when a field is edited.

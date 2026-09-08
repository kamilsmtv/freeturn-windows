"""Генератор иконки FreeTurn (вариант A: ромб с ядром).

Геометрия взята из макета: контурный ромб и ромбовидная сердцевина -
та же идея, что у иконки Android-клиента, но в акцентном цвете приложения
и с более тонким контуром. Мелкие размеры рисуются не уменьшением
большого, а собственными пропорциями: иначе контур в 16 px замыливается.
"""

from PIL import Image, ImageDraw

TILE = (0x18, 0x18, 0x1B, 255)      # zinc-900, как поверхности в приложении
ACCENT = (0x10, 0xB9, 0x81, 255)    # emerald-500
MUTED = (0x52, 0x52, 0x5B, 255)     # zinc-600, «отключено» на светлой панели
MUTED_LIGHT = (0xD4, 0xD4, 0xD8, 255)  # zinc-300, «отключено» на тёмной панели
DANGER = (0xF8, 0x71, 0x71, 255)    # red-400, состояние «ошибка»

SS = 8  # сглаживание сверхдискретизацией


def geometry(size):
    """Толщина контура и размер ядра в долях глифа: чем меньше иконка,
    тем толще штрих, иначе он исчезает."""
    if size >= 48:
        return 16 / 256, 40 / 256
    if size >= 32:
        return 18 / 256, 42 / 256
    return 22 / 256, 46 / 256


def diamond(cx, cy, r):
    return [(cx, cy - r), (cx + r, cy), (cx, cy + r), (cx - r, cy)]


def draw_icon(size, accent=ACCENT, tile=TILE, glyph_ratio=0.75):
    px = size * SS
    img = Image.new("RGBA", (px, px), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    if tile is not None:
        d.rounded_rectangle([0, 0, px - 1, px - 1], radius=px * 0.22, fill=tile)

    glyph = px * glyph_ratio
    cx = cy = px / 2
    stroke_k, core_k = geometry(size)

    r_out = glyph * (104 / 256)
    # У ромба перпендикулярная толщина в √2 раз меньше разницы радиусов.
    r_in = r_out - glyph * stroke_k * (2 ** 0.5)

    d.polygon(diamond(cx, cy, r_out), fill=accent)
    d.polygon(diamond(cx, cy, r_in), fill=(0, 0, 0, 0))
    if tile is not None:
        d.polygon(diamond(cx, cy, r_in), fill=tile)
    d.polygon(diamond(cx, cy, glyph * core_k), fill=accent)

    return img.resize((size, size), Image.LANCZOS)


def write_ico(path, images):
    """ICO с PNG-содержимым: так поддерживаются все размеры без палитр."""
    import struct
    from io import BytesIO

    blobs = []
    for img in images:
        buf = BytesIO()
        img.save(buf, format="PNG")
        blobs.append(buf.getvalue())

    out = bytearray()
    out += struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    for img, blob in zip(images, blobs):
        dim = 0 if img.width >= 256 else img.width
        out += struct.pack("<BBBBHHII", dim, dim, 0, 0, 1, 32, len(blob), offset)
        offset += len(blob)
    for blob in blobs:
        out += blob

    with open(path, "wb") as f:
        f.write(out)


if __name__ == "__main__":
    import sys

    outdir = sys.argv[1] if len(sys.argv) > 1 else "."
    sizes = [16, 20, 24, 32, 48, 64, 128, 256]

    app = [draw_icon(s) for s in sizes]
    for s, img in zip(sizes, app):
        img.save(f"{outdir}/icon-{s}.png")
    write_ico(f"{outdir}/icon.ico", app)

    # Глиф без подложки - для трея: подложка там мешает, а цвет несёт состояние.
    tray_sizes = (16, 20, 24, 32)
    # Для «отключено» нужны два варианта: серый глиф теряется на панели
    # задач того же тона, а тему панели приложение выбирает в рантайме.
    for name, color in (
        ("tray", ACCENT),
        ("tray-off", MUTED),
        ("tray-off-light", MUTED_LIGHT),
        ("tray-error", DANGER),
    ):
        write_ico(
            f"{outdir}/{name}.ico",
            [draw_icon(s, accent=color, tile=None, glyph_ratio=0.92) for s in tray_sizes],
        )
        for s in tray_sizes:
            draw_icon(s, accent=color, tile=None, glyph_ratio=0.92).save(f"{outdir}/{name}-{s}.png")

    print("готово:", ", ".join(f"icon-{s}.png" for s in sizes),
          "+ icon.ico, tray.ico, tray-off.ico, tray-off-light.ico, tray-error.ico")

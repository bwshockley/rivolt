import { describe, expect, it } from "vitest";
import L from "leaflet";
import { addOSMTiles, applyOSMTheme, OSM_TILE_ATTRIBUTION, OSM_TILE_URL } from "./osmTiles";

describe("OpenStreetMap Leaflet tiles", () => {
  it("adds the public OSM tile layer with attribution and a supported max zoom", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const map = L.map(el).setView([40, -100], 5);
    const tiles = addOSMTiles(map);

    expect(tiles).toBeInstanceOf(L.TileLayer);
    expect(OSM_TILE_URL).toBe("https://tile.openstreetmap.org/{z}/{x}/{y}.png");
    expect(tiles.options.attribution).toBe(OSM_TILE_ATTRIBUTION);
    expect(tiles.options.maxZoom).toBe(19);
    expect(map.hasLayer(tiles)).toBe(true);

    map.remove();
    el.remove();
  });

  it("filters only the tile pane to follow the selected map theme", () => {
    const el = document.createElement("div");
    document.body.appendChild(el);
    const map = L.map(el).setView([40, -100], 5);
    addOSMTiles(map);

    applyOSMTheme(map, "dark");
    expect(map.getPane("tilePane")?.style.filter).toContain("invert(1)");
    expect(map.getContainer().style.background).toBe("rgb(10, 10, 10)");

    applyOSMTheme(map, "light");
    expect(map.getPane("tilePane")?.style.filter).toBe("");
    expect(map.getContainer().style.background).toBe("rgb(229, 229, 229)");

    map.remove();
    el.remove();
  });
});
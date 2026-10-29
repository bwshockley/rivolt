import L from "leaflet";
import type { MapFlavor } from "./mapTheme";

export const OSM_TILE_URL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
export const OSM_TILE_ATTRIBUTION =
  '© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap contributors</a>';

const DARK_TILE_FILTER = "invert(1) hue-rotate(180deg) saturate(0.65)";

export function addOSMTiles(map: L.Map): L.TileLayer {
  return L.tileLayer(OSM_TILE_URL, {
    attribution: OSM_TILE_ATTRIBUTION,
    maxZoom: 19,
  }).addTo(map);
}

export function applyOSMTheme(map: L.Map, flavor: MapFlavor): void {
  const dark = flavor === "dark";
  map.getContainer().style.background = dark ? "#0a0a0a" : "#e5e5e5";
  const tilePane = map.getPane("tilePane");
  if (tilePane) tilePane.style.filter = dark ? DARK_TILE_FILTER : "";
}
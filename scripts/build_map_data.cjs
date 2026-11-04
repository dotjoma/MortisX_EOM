const fs = require('fs');
const path = require('path');

const root = 'D:/! GITHUB PROJECTS/MadrigalMultibot';
const dumpDir = path.join(root, 'madrigal_dump');
const outPath = path.join(root, 'src/features/map-tab/mapData.json');

const movers = JSON.parse(fs.readFileSync(path.join(dumpDir, 'world/movers.json'), 'utf8'));
const gameCore = JSON.parse(fs.readFileSync(path.join(dumpDir, 'world/game_core.json'), 'utf8'));

const shops = new Set(Object.keys(gameCore.shops || {}));
const monstersMap = gameCore.monsters || {};

// 1. NPCs
const npcs = [];
const seenNpcs = new Set();
for (const o of (movers.npcs || [])) {
  let nm = String(o.name || o.key || '').trim();
  if (!nm) continue;
  if (nm.startsWith('MaFl_') || nm.startsWith('Mafl_') || nm.startsWith('MaDa_') || nm.startsWith('MaSa_') || nm.startsWith('MaMa_')) {
    nm = nm.substring(5);
  }
  const key = String(o.key || '');
  const isShop = shops.has(key);
  const p = o.p || [0, 0, 0];
  const x = Math.round(p[0] * 10) / 10;
  const z = Math.round(p[2] * 10) / 10;

  const dedupKey = `${nm}_${Math.round(x)}_${Math.round(z)}`;
  if (seenNpcs.has(dedupKey)) continue;
  seenNpcs.add(dedupKey);

  npcs.push({
    name: nm,
    key,
    x,
    z,
    shop: isShop
  });
}

// 2. Monster Spawns
// Aggregate spawns by (mi + grid cluster) or keep distinct areas
const monsterAreas = [];
for (const s of (movers.spawns || [])) {
  const mon = monstersMap[s.mi];
  if (!mon) continue;
  const rect = s.rect || [0, 0, 0, 0];
  const x1 = rect[0], z1 = rect[1], x2 = rect[2], z2 = rect[3];
  const cx = Math.round(((x1 + x2) / 2) * 10) / 10;
  const cz = Math.round(((z1 + z2) / 2) * 10) / 10;
  const r = Math.round((Math.max(Math.abs(x2 - x1), Math.abs(z2 - z1)) * 0.5) * 10) / 10;
  const isBoss = mon.rank === 'RANK_SUPER' || mon.rank === 'RANK_BOSS' || s.n === 1 && s.respawn >= 1000;

  monsterAreas.push({
    mi: s.mi,
    name: mon.name || s.m,
    lv: mon.lv || 1,
    x: cx,
    z: cz,
    r: Math.max(10, r),
    n: s.n || 1,
    boss: isBoss
  });
}

// 3. Towns & Named Regions
const regions = [];
for (const r of (gameCore.regions || [])) {
  const rect = r.rect || [0, 0, 0, 0];
  const cx = Math.round(((rect[0] + rect[2]) / 2) * 10) / 10;
  // in game_core.json, regions rect is [x1, y1, x2, y2] where y is positive distance corresponding to -z
  const cy = Math.round(((rect[1] + rect[3]) / 2) * 10) / 10;
  // in 3D world coords, z = -cy
  const z = -cy;
  regions.push({
    name: String(r.name || ''),
    x: cx,
    z,
    town: Boolean(r.town)
  });
}

// 4. Revive / Lodestars
const lodestars = [];
for (const rev of (gameCore.revive || [])) {
  // rev has key, x, z (in game_core: x and z are positive map plane, where 3D z = -rev.z)
  lodestars.push({
    name: `Lodestar (${rev.key})`,
    x: Math.round(rev.x * 10) / 10,
    z: -Math.round(rev.z * 10) / 10
  });
}

const mapData = {
  bounds: {
    originX: 1536.0,
    originY: 5120.0,
    texPpm: 0.375,
    width: 3264,
    height: 1344,
    townFlarine: {
      tl: [6850.3, -3396.8],
      br: [7259.4, -3125.5]
    }
  },
  npcs,
  monsterAreas,
  regions,
  lodestars
};

fs.writeFileSync(outPath, JSON.stringify(mapData, null, 2));
console.log(`Generated ${outPath}:`, {
  npcs: npcs.length,
  monsterAreas: monsterAreas.length,
  regions: regions.length,
  lodestars: lodestars.length
});

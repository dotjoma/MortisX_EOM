import React, { useEffect, useRef, useState, useMemo, useCallback } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { useMultiSelection, multiRunCommand } from '../../multi';
import rawMapData from './mapData.json';

interface MapTabProps {
  bot?: BotSummary | null;
  allBots?: BotSummary[];
}

interface MapEntity {
  name: string;
  x: number;
  z: number;
  type: 'Mob' | 'NPC' | 'Shop' | 'Town' | 'Lodestar' | 'Bot';
  sub?: string;
  dist?: number;
}

const mapData = rawMapData as {
  bounds: {
    originX: number;
    originY: number;
    texPpm: number;
    width: number;
    height: number;
    townFlarine: { tl: [number, number]; br: [number, number] };
  };
  npcs: Array<{ name: string; key: string; x: number; z: number; shop: boolean }>;
  monsterAreas: Array<{ mi: string; name: string; lv: number; x: number; z: number; r: number; n: number; boss: boolean }>;
  regions: Array<{ name: string; x: number; z: number; town: boolean }>;
  lodestars: Array<{ name: string; x: number; z: number }>;
};

// Coordinate projection formulas
function worldToTex(x: number, z: number) {
  return {
    tx: (x - 1536.0) * 0.375,
    ty: (5120.0 + z) * 0.375,
  };
}

function texToWorld(tx: number, ty: number) {
  return {
    x: tx / 0.375 + 1536.0,
    z: ty / 0.375 - 5120.0,
  };
}

function texToScreen(tx: number, ty: number, camX: number, camY: number, zoom: number, cw: number, ch: number) {
  return {
    sx: cw * 0.5 + (tx - camX) * zoom,
    sy: ch * 0.5 + (ty - camY) * zoom,
  };
}

function screenToTex(sx: number, sy: number, camX: number, camY: number, zoom: number, cw: number, ch: number) {
  return {
    tx: camX + (sx - cw * 0.5) / zoom,
    ty: camY + (sy - ch * 0.5) / zoom,
  };
}

function worldToScreen(x: number, z: number, camX: number, camY: number, zoom: number, cw: number, ch: number) {
  const { tx, ty } = worldToTex(x, z);
  return texToScreen(tx, ty, camX, camY, zoom, cw, ch);
}

function screenToWorld(sx: number, sy: number, camX: number, camY: number, zoom: number, cw: number, ch: number) {
  const { tx, ty } = screenToTex(sx, sy, camX, camY, zoom, cw, ch);
  return texToWorld(tx, ty);
}

function getMobLevelColor(mobLv: number, playerLv: number): { color: string; label: string } {
  const diff = mobLv - playerLv;
  if (diff <= -10) return { color: '#8a94a0', label: 'Very Low' };
  if (diff <= -3) return { color: '#3ddc97', label: 'Easy' };
  if (diff <= 2) return { color: '#ffe066', label: 'Even' };
  if (diff <= 6) return { color: '#ff9a4a', label: 'Hard' };
  return { color: '#ff4a4a', label: 'Deadly' };
}

// Module-level cached singletons for map textures
let globalWorldImg: HTMLImageElement | null = null;
let globalTownImg: HTMLImageElement | null = null;
let imagesLoaded = false;
const imageListeners = new Set<() => void>();

function preloadMapImages() {
  if (imagesLoaded) return;
  let worldReady = false;
  let townReady = false;

  const notify = () => {
    if (worldReady && townReady) {
      imagesLoaded = true;
      imageListeners.forEach((cb) => cb());
    }
  };

  const loadImg = (src: string, onReady: () => void, maxRetries = 5) => {
    let attempts = 0;
    const img = new Image();
    const tryLoad = () => {
      attempts++;
      img.src = src;
    };
    img.onload = () => {
      if (img.naturalWidth > 0) {
        onReady();
      } else if (attempts < maxRetries) {
        setTimeout(tryLoad, 1500);
      }
    };
    img.onerror = () => {
      if (attempts < maxRetries) {
        setTimeout(tryLoad, 1500);
      }
    };
    tryLoad();
    return img;
  };

  if (!globalWorldImg) {
    globalWorldImg = loadImg('/maps/minimap_madrigal.png', () => {
      worldReady = true;
      notify();
    });
  } else if (globalWorldImg.naturalWidth > 0) {
    worldReady = true;
  }

  if (!globalTownImg) {
    globalTownImg = loadImg('/maps/town_flarine.png', () => {
      townReady = true;
      notify();
    });
  } else if (globalTownImg.naturalWidth > 0) {
    townReady = true;
  }

  notify();
}

export const MapTab: React.FC<MapTabProps> = ({ bot, allBots = [] }) => {
  const { target } = useMultiSelection();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  // Images
  const [imagesReady, setImagesReady] = useState(() => imagesLoaded);

  useEffect(() => {
    if (imagesLoaded) {
      setImagesReady(true);
      return;
    }
    const onLoaded = () => setImagesReady(true);
    imageListeners.add(onLoaded);
    preloadMapImages();
    return () => {
      imageListeners.delete(onLoaded);
    };
  }, []);

  // Viewport camera state (in texture coordinates)
  const [camX, setCamX] = useState<number>(2034); // Flarine center
  const [camY, setCamY] = useState<number>(682.5);
  const [zoom, setZoom] = useState<number>(1.0);
  const [canvasSize, setCanvasSize] = useState({ width: 800, height: 600 });

  // Interactions
  const [isDragging, setIsDragging] = useState(false);
  const [dragMoved, setDragMoved] = useState(false);
  const dragStart = useRef({ x: 0, y: 0, camX: 0, camY: 0 });
  const [cursorPos, setCursorPos] = useState<{ x: number; z: number } | null>(null);
  const [hoveredEntity, setHoveredEntity] = useState<MapEntity | null>(null);
  const [waypoint, setWaypoint] = useState<{ x: number; z: number } | null>(null);

  // Active waypoint: prioritize local state or synced bot stats
  const activeWaypoint = useMemo(() => {
    if (waypoint) return waypoint;
    if (bot?.stats?.waypoint) return { x: bot.stats.waypoint.x, z: bot.stats.waypoint.z };
    return null;
  }, [waypoint, bot?.stats?.waypoint]);

  // Auto-clear local waypoint when arrived
  useEffect(() => {
    if (waypoint && bot?.stats) {
      const d = Math.hypot(waypoint.x - bot.stats.pos_x, waypoint.z - bot.stats.pos_z);
      if (d <= 2.2) {
        setWaypoint(null);
      }
    }
  }, [bot?.stats?.pos_x, bot?.stats?.pos_z, waypoint]);

  // Search & Filter UI state
  const [showSearchMenu, setShowSearchMenu] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [layers, setLayers] = useState({
    monsters: true,
    npcs: true,
    towns: true,
    lodestars: true,
    otherBots: true,
    labels: true,
  });

  // Track initial auto-focus
  const hasAutoCentered = useRef(false);
  const prevBotId = useRef<string | null>(null);

  // Center on current bot upon first load or selection
  useEffect(() => {
    if (bot?.id !== prevBotId.current) {
      hasAutoCentered.current = false;
      prevBotId.current = bot?.id || null;
    }
    if (bot && bot.status === 'online' && !hasAutoCentered.current) {
      const { tx, ty } = worldToTex(bot.stats.pos_x, bot.stats.pos_z);
      setCamX(tx);
      setCamY(ty);
      setZoom(2.0);
      hasAutoCentered.current = true;
    }
  }, [bot]);

  // Center on bot helper
  const handleCenterOnBot = useCallback(() => {
    if (!bot) return;
    const { tx, ty } = worldToTex(bot.stats.pos_x, bot.stats.pos_z);
    setCamX(tx);
    setCamY(ty);
    setZoom(2.0);
  }, [bot]);

  // Reset to full world view (dynamically fits screen)
  const handleResetWorldView = useCallback(() => {
    setCamX(mapData.bounds.width * 0.5);
    setCamY(mapData.bounds.height * 0.5);
    const fitZoom = Math.min(canvasSize.width / 3264, canvasSize.height / 1344) * 0.96;
    setZoom(Math.max(0.15, fitZoom));
  }, [canvasSize]);

  // Send waypoint navigation command to in-game bot
  const handleWaypointSelect = useCallback((x: number, z: number, isWarp = false) => {
    setWaypoint({ x, z });
    if (isWarp) {
      void multiRunCommand(target, 'map.nav.warp', { x, z });
    } else {
      void multiRunCommand(target, 'map.nav.waypoint_set', { x, z });
    }
  }, [target]);

  // Cancel waypoint navigation and stop in-game bot
  const handleWaypointClear = useCallback(() => {
    setWaypoint(null);
    void multiRunCommand(target, 'map.nav.waypoint_clear', {});
  }, [target]);

  // Jump to specific world coordinates
  const handleJumpToCoords = useCallback((x: number, z: number, zoomLevel = 2.2) => {
    const { tx, ty } = worldToTex(x, z);
    setCamX(tx);
    setCamY(ty);
    setZoom(zoomLevel);
    handleWaypointSelect(x, z);
  }, [handleWaypointSelect]);

  // ResizeObserver for canvas container
  useEffect(() => {
    if (!containerRef.current) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0) {
          setCanvasSize({ width: Math.round(width), height: Math.round(height) });
        }
      }
    });
    ro.observe(containerRef.current);
    return () => ro.disconnect();
  }, []);

  // Canvas Mouse Events
  const handleMouseDown = (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (e.button === 0) {
      setIsDragging(true);
      setDragMoved(false);
      dragStart.current = {
        x: e.clientX,
        y: e.clientY,
        camX,
        camY,
      };
    } else if (e.button === 2) {
      // Right-click: clear waypoint & stop bot in-game
      e.preventDefault();
      handleWaypointClear();
    }
  };

  const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = canvasRef.current?.getBoundingClientRect();
    if (!rect) return;
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    if (isDragging) {
      const dx = e.clientX - dragStart.current.x;
      const dy = e.clientY - dragStart.current.y;
      if (Math.hypot(dx, dy) > 3) {
        setDragMoved(true);
        setCamX(dragStart.current.camX - dx / zoom);
        setCamY(dragStart.current.camY - dy / zoom);
      }
    }

    const world = screenToWorld(mx, my, camX, camY, zoom, rect.width, rect.height);
    setCursorPos({ x: Math.round(world.x), z: Math.round(world.z) });

    // Entity hover detection
    let found: MapEntity | null = null;
    const cw = rect.width;
    const ch = rect.height;

    // Check NPCs
    if (layers.npcs) {
      for (const n of mapData.npcs) {
        const s = worldToScreen(n.x, n.z, camX, camY, zoom, cw, ch);
        if (Math.hypot(s.sx - mx, s.sy - my) <= 10) {
          found = { name: n.name, x: n.x, z: n.z, type: n.shop ? 'Shop' : 'NPC', sub: n.shop ? 'Shopkeeper' : 'NPC' };
          break;
        }
      }
    }

    // Check Mobs
    if (!found && layers.monsters) {
      for (const m of mapData.monsterAreas) {
        const s = worldToScreen(m.x, m.z, camX, camY, zoom, cw, ch);
        const sr = Math.max(8, m.r * 0.375 * zoom);
        if (Math.hypot(s.sx - mx, s.sy - my) <= Math.min(sr, 20)) {
          found = {
            name: m.name,
            x: m.x,
            z: m.z,
            type: 'Mob',
            sub: `Lv.${m.lv} (${m.n} spawns)${m.boss ? ' - Boss' : ''}`,
          };
          break;
        }
      }
    }

    // Check Other Bots
    if (!found && layers.otherBots) {
      for (const ob of allBots) {
        if (ob.id === bot?.id) continue;
        if (ob.status !== 'online') continue;
        const s = worldToScreen(ob.stats.pos_x, ob.stats.pos_z, camX, camY, zoom, cw, ch);
        if (Math.hypot(s.sx - mx, s.sy - my) <= 12) {
          found = {
            name: ob.character_name || ob.name,
            x: ob.stats.pos_x,
            z: ob.stats.pos_z,
            type: 'Bot',
            sub: `Lv.${ob.stats.level || 1} ${ob.stats.job || 'Player'}`,
          };
          break;
        }
      }
    }

    setHoveredEntity(found);
  };

  const handleMouseUp = (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (e.button === 0) {
      setIsDragging(false);
      if (!dragMoved) {
        const rect = canvasRef.current?.getBoundingClientRect();
        if (rect) {
          let targetX: number;
          let targetZ: number;
          if (hoveredEntity) {
            targetX = hoveredEntity.x;
            targetZ = hoveredEntity.z;
          } else {
            const mx = e.clientX - rect.left;
            const my = e.clientY - rect.top;
            const world = screenToWorld(mx, my, camX, camY, zoom, rect.width, rect.height);
            targetX = Math.round(world.x * 10) / 10;
            targetZ = Math.round(world.z * 10) / 10;
          }
          handleWaypointSelect(targetX, targetZ, e.shiftKey);
        }
      }
    }
  };

  const handleMouseLeave = () => {
    setIsDragging(false);
    setCursorPos(null);
    setHoveredEntity(null);
  };

  const handleWheel = (e: React.WheelEvent<HTMLCanvasElement>) => {
    e.preventDefault();
    const rect = canvasRef.current?.getBoundingClientRect();
    if (!rect) return;
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    const factor = e.deltaY < 0 ? 1.25 : 1 / 1.25;
    const newZoom = Math.min(10.0, Math.max(0.15, zoom * factor));
    if (newZoom === zoom) return;

    // Anchor zoom at mouse pointer
    const beforeTex = screenToTex(mx, my, camX, camY, zoom, rect.width, rect.height);
    const newCamX = beforeTex.tx - (mx - rect.width * 0.5) / newZoom;
    const newCamY = beforeTex.ty - (my - rect.height * 0.5) / newZoom;

    setCamX(newCamX);
    setCamY(newCamY);
    setZoom(newZoom);
  };

  const lastRenderTimeRef = useRef<number>(0);
  const renderStateRef = useRef({
    bot,
    allBots,
    camX,
    camY,
    zoom,
    layers,
    activeWaypoint,
    hoveredEntity,
  });

  renderStateRef.current = {
    bot,
    allBots,
    camX,
    camY,
    zoom,
    layers,
    activeWaypoint,
    hoveredEntity,
  };

  // Main Canvas Render Loop
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let animId: number;
    const FRAME_INTERVAL = 50;

    const render = (time: number) => {
      animId = requestAnimationFrame(render);
      if (time - lastRenderTimeRef.current < FRAME_INTERVAL) {
        return;
      }
      lastRenderTimeRef.current = time;

      const {
        bot,
        allBots,
        camX,
        camY,
        zoom,
        layers,
        activeWaypoint,
        hoveredEntity,
      } = renderStateRef.current;

      const cw = canvasSize.width;
      const ch = canvasSize.height;
      const dpr = window.devicePixelRatio || 1;

      ctx.save();
      ctx.scale(dpr, dpr);

      // Background
      ctx.fillStyle = '#080203';
      ctx.fillRect(0, 0, cw, ch);

      // 1. Draw World Map Texture
      const worldImg = globalWorldImg;
      if (imagesReady && worldImg && worldImg.complete && worldImg.naturalWidth > 0) {
        const left = cw * 0.5 - camX * zoom;
        const top = ch * 0.5 - camY * zoom;
        const w = 3264 * zoom;
        const h = 1344 * zoom;
        ctx.drawImage(worldImg, left, top, w, h);
      } else {
        // Fallback grid & loading indicator
        ctx.strokeStyle = '#220b0e';
        ctx.lineWidth = 1;
        for (let x = 0; x < cw; x += 40) {
          ctx.beginPath();
          ctx.moveTo(x, 0);
          ctx.lineTo(x, ch);
          ctx.stroke();
        }
        for (let y = 0; y < ch; y += 40) {
          ctx.beginPath();
          ctx.moveTo(0, y);
          ctx.lineTo(cw, y);
          ctx.stroke();
        }

        ctx.font = '12px "Segoe UI", sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = 'var(--sw, #e5a83c)';
        ctx.fillText('Loading Madrigal World Map...', cw * 0.5, ch * 0.5);
      }

      // 2. Flarine Town Overlay (seamless blend on zoom in)
      const townImg = globalTownImg;
      if (zoom > 1.2 && townImg && townImg.complete && townImg.naturalWidth > 0) {
        const flarineTL = worldToTex(mapData.bounds.townFlarine.tl[0], mapData.bounds.townFlarine.tl[1]);
        const flarineBR = worldToTex(mapData.bounds.townFlarine.br[0], mapData.bounds.townFlarine.br[1]);
        const sTL = texToScreen(flarineTL.tx, flarineTL.ty, camX, camY, zoom, cw, ch);
        const sBR = texToScreen(flarineBR.tx, flarineBR.ty, camX, camY, zoom, cw, ch);

        const fw = sBR.sx - sTL.sx;
        const fh = sBR.sy - sTL.sy;
        const alpha = Math.min(1.0, Math.max(0.0, (zoom - 1.2) / 0.8));

        ctx.save();
        ctx.globalAlpha = alpha;
        ctx.drawImage(townImg, sTL.sx, sTL.sy, fw, fh);
        ctx.restore();
      }

      const playerLv = bot?.stats.level || 1;

      // 3. Draw Monster Spawn Areas
      if (layers.monsters) {
        for (const m of mapData.monsterAreas) {
          const s = worldToScreen(m.x, m.z, camX, camY, zoom, cw, ch);
          const rad = Math.max(4, Math.min(50, m.r * 0.375 * zoom));

          // Screen culling
          if (s.sx < -rad || s.sx > cw + rad || s.sy < -rad || s.sy > ch + rad) continue;

          const { color } = getMobLevelColor(m.lv, playerLv);

          // Translucent fill
          ctx.beginPath();
          ctx.arc(s.sx, s.sy, rad, 0, Math.PI * 2);
          ctx.fillStyle = color + (m.boss ? '35' : '18');
          ctx.fill();

          // Border ring
          ctx.strokeStyle = color + (m.boss ? 'cc' : '66');
          ctx.lineWidth = m.boss ? 2 : 1;
          ctx.stroke();

          // Center point
          ctx.beginPath();
          ctx.arc(s.sx, s.sy, m.boss ? 4 : 2, 0, Math.PI * 2);
          ctx.fillStyle = color;
          ctx.fill();

          // Labels when zoomed in or on boss
          if (layers.labels && (zoom >= 1.5 || (m.boss && zoom >= 0.8))) {
            ctx.font = '10px "Segoe UI", sans-serif';
            ctx.textAlign = 'center';
            ctx.fillStyle = '#000000';
            ctx.fillText(`${m.name} Lv.${m.lv}`, s.sx + 1, s.sy - rad - 3);
            ctx.fillStyle = color;
            ctx.fillText(`${m.name} Lv.${m.lv}`, s.sx, s.sy - rad - 4);
          }
        }
      }

      // 4. Draw Lodestars
      if (layers.lodestars) {
        for (const l of mapData.lodestars) {
          const s = worldToScreen(l.x, l.z, camX, camY, zoom, cw, ch);
          if (s.sx < -10 || s.sx > cw + 10 || s.sy < -10 || s.sy > ch + 10) continue;

          // Cross marker
          ctx.fillStyle = '#ffffff';
          ctx.fillRect(s.sx - 1.5, s.sy - 5, 3, 10);
          ctx.fillRect(s.sx - 5, s.sy - 1.5, 10, 3);

          if (layers.labels && zoom >= 1.2) {
            ctx.font = '10px "Segoe UI", sans-serif';
            ctx.textAlign = 'center';
            ctx.fillStyle = '#ffffff';
            ctx.fillText(l.name, s.sx, s.sy - 7);
          }
        }
      }

      // 5. Draw NPCs
      if (layers.npcs) {
        for (const n of mapData.npcs) {
          const s = worldToScreen(n.x, n.z, camX, camY, zoom, cw, ch);
          if (s.sx < -10 || s.sx > cw + 10 || s.sy < -10 || s.sy > ch + 10) continue;

          const col = n.shop ? '#ffd84d' : '#6fb6ff';

          // Outer shadow & inner dot
          ctx.beginPath();
          ctx.arc(s.sx, s.sy, 3.5, 0, Math.PI * 2);
          ctx.fillStyle = 'rgba(0,0,0,0.7)';
          ctx.fill();

          ctx.beginPath();
          ctx.arc(s.sx, s.sy, 2.5, 0, Math.PI * 2);
          ctx.fillStyle = col;
          ctx.fill();

          if (layers.labels && zoom >= 2.0) {
            ctx.font = '10px "Segoe UI", sans-serif';
            ctx.textAlign = 'center';
            ctx.fillStyle = '#000000';
            ctx.fillText(n.name, s.sx + 1, s.sy - 6);
            ctx.fillStyle = col;
            ctx.fillText(n.name, s.sx, s.sy - 7);
          }
        }
      }

      // 6. Draw Region / Town Labels
      if (layers.towns && layers.labels) {
        for (const r of mapData.regions) {
          if (!r.town && zoom < 1.2) continue;
          const s = worldToScreen(r.x, r.z, camX, camY, zoom, cw, ch);
          if (s.sx < -50 || s.sx > cw + 50 || s.sy < -20 || s.sy > ch + 20) continue;

          ctx.font = r.town ? 'bold 12px "Segoe UI", sans-serif' : '11px "Segoe UI", sans-serif';
          ctx.textAlign = 'center';
          ctx.fillStyle = 'rgba(0, 0, 0, 0.85)';
          ctx.fillText(r.name, s.sx + 1, s.sy + 1);
          ctx.fillStyle = r.town ? '#f0e4df' : '#a98d8d';
          ctx.fillText(r.name, s.sx, s.sy);
        }
      }

      // 7. Draw Other Bots
      if (layers.otherBots) {
        for (const ob of allBots) {
          if (ob.id === bot?.id) continue;
          if (ob.status !== 'online') continue;
          const s = worldToScreen(ob.stats.pos_x, ob.stats.pos_z, camX, camY, zoom, cw, ch);
          if (s.sx < -20 || s.sx > cw + 20 || s.sy < -20 || s.sy > ch + 20) continue;

          const isSelected = target.ids.includes(ob.id);
          const color = isSelected ? '#58d5ff' : '#3ddc97';

          ctx.beginPath();
          ctx.arc(s.sx, s.sy, isSelected ? 6.5 : 5, 0, Math.PI * 2);
          ctx.fillStyle = 'rgba(0, 0, 0, 0.8)';
          ctx.fill();

          ctx.beginPath();
          ctx.arc(s.sx, s.sy, isSelected ? 5 : 3.8, 0, Math.PI * 2);
          ctx.fillStyle = color;
          ctx.fill();

          ctx.font = 'bold 10px "Segoe UI", sans-serif';
          ctx.textAlign = 'center';
          ctx.fillStyle = '#000000';
          ctx.fillText(ob.character_name || ob.name, s.sx + 1, s.sy - 7);
          ctx.fillStyle = color;
          ctx.fillText(ob.character_name || ob.name, s.sx, s.sy - 8);
        }
      }

      // 8. Draw Active Selected Bot Marker
      if (bot) {
        const s = worldToScreen(bot.stats.pos_x, bot.stats.pos_z, camX, camY, zoom, cw, ch);
        const time = Date.now();
        const pulse = 8 + Math.sin(time / 200) * 3;

        // Pulsing radar ring
        ctx.beginPath();
        ctx.arc(s.sx, s.sy, pulse, 0, Math.PI * 2);
        ctx.strokeStyle = 'rgba(255, 204, 0, 0.6)';
        ctx.lineWidth = 1.5;
        ctx.stroke();

        // Direction cone/triangle
        ctx.beginPath();
        ctx.moveTo(s.sx, s.sy - 10);
        ctx.lineTo(s.sx + 6, s.sy + 6);
        ctx.lineTo(s.sx, s.sy + 3);
        ctx.lineTo(s.sx - 6, s.sy + 6);
        ctx.closePath();
        ctx.fillStyle = '#ffd84d';
        ctx.fill();
        ctx.strokeStyle = '#000000';
        ctx.lineWidth = 1;
        ctx.stroke();

        // Bot Label Tag
        const botTitle = `${bot.character_name || bot.name} (Lv.${bot.stats.level || 1})`;
        ctx.font = 'bold 11px "Segoe UI", sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#000000';
        ctx.fillText(botTitle, s.sx + 1, s.sy - 13);
        ctx.fillStyle = '#ffd84d';
        ctx.fillText(botTitle, s.sx, s.sy - 14);
      }

      // 9. Draw Waypoint Flag & Path
      if (activeWaypoint) {
        const wp = worldToScreen(activeWaypoint.x, activeWaypoint.z, camX, camY, zoom, cw, ch);

        // Dashed path line from bot to waypoint
        if (bot) {
          const bp = worldToScreen(bot.stats.pos_x, bot.stats.pos_z, camX, camY, zoom, cw, ch);
          ctx.beginPath();
          ctx.setLineDash([5, 5]);
          ctx.moveTo(bp.sx, bp.sy);
          ctx.lineTo(wp.sx, wp.sy);
          ctx.strokeStyle = 'rgba(255, 92, 108, 0.85)';
          ctx.lineWidth = 2;
          ctx.stroke();
          ctx.setLineDash([]);

          // Distance label on path
          const dist = Math.round(Math.hypot(activeWaypoint.x - bot.stats.pos_x, activeWaypoint.z - bot.stats.pos_z));
          const midX = (bp.sx + wp.sx) * 0.5;
          const midY = (bp.sy + wp.sy) * 0.5;
          ctx.font = 'bold 11px "Segoe UI", sans-serif';
          ctx.textAlign = 'center';
          ctx.fillStyle = '#000000';
          ctx.fillText(`${dist}m`, midX + 1, midY - 4);
          ctx.fillStyle = '#ff5c6c';
          ctx.fillText(`${dist}m`, midX, midY - 5);
        }

        // Flag pole & banner
        ctx.beginPath();
        ctx.moveTo(wp.sx, wp.sy);
        ctx.lineTo(wp.sx, wp.sy - 18);
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 2;
        ctx.stroke();

        ctx.beginPath();
        ctx.moveTo(wp.sx, wp.sy - 18);
        ctx.lineTo(wp.sx + 12, wp.sy - 13);
        ctx.lineTo(wp.sx, wp.sy - 8);
        ctx.closePath();
        ctx.fillStyle = '#ff5c6c';
        ctx.fill();

        ctx.font = '10px "Segoe UI", sans-serif';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#ffffff';
        ctx.fillText('Waypoint', wp.sx, wp.sy + 12);
      }

      // 10. Hover Tooltip
      if (hoveredEntity) {
        const s = worldToScreen(hoveredEntity.x, hoveredEntity.z, camX, camY, zoom, cw, ch);
        const text = `${hoveredEntity.name} ${hoveredEntity.sub ? `• ${hoveredEntity.sub}` : ''}`;
        ctx.font = 'bold 11px "Segoe UI", sans-serif';
        const tw = ctx.measureText(text).width;

        const bx = Math.min(cw - tw - 16, Math.max(8, s.sx - tw * 0.5 - 6));
        const by = s.sy - 28 < 20 ? s.sy + 16 : s.sy - 28;

        ctx.fillStyle = 'rgba(12, 3, 4, 0.9)';
        ctx.strokeStyle = 'var(--ln, #4a171b)';
        ctx.lineWidth = 1;
        ctx.fillRect(bx, by, tw + 12, 20);
        ctx.strokeRect(bx, by, tw + 12, 20);

        ctx.textAlign = 'left';
        ctx.fillStyle = '#f0e4df';
        ctx.fillText(text, bx + 6, by + 14);
      }

      ctx.restore();
    };

    animId = requestAnimationFrame(render);
    return () => cancelAnimationFrame(animId);
  }, [canvasSize, imagesReady]);

  // Search Results
  const searchResults = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return [];

    const bx = bot?.stats.pos_x || 6960;
    const bz = bot?.stats.pos_z || -3300;

    const results: Array<MapEntity> = [];

    for (const r of mapData.regions) {
      if (r.name.toLowerCase().includes(q)) {
        results.push({
          name: r.name,
          x: r.x,
          z: r.z,
          type: 'Town',
          sub: r.town ? 'Town' : 'Area',
          dist: Math.round(Math.hypot(r.x - bx, r.z - bz)),
        });
      }
    }

    for (const n of mapData.npcs) {
      if (n.name.toLowerCase().includes(q)) {
        results.push({
          name: n.name,
          x: n.x,
          z: n.z,
          type: n.shop ? 'Shop' : 'NPC',
          sub: n.shop ? 'Shop' : 'NPC',
          dist: Math.round(Math.hypot(n.x - bx, n.z - bz)),
        });
      }
    }

    for (const m of mapData.monsterAreas) {
      if (m.name.toLowerCase().includes(q)) {
        results.push({
          name: m.name,
          x: m.x,
          z: m.z,
          type: 'Mob',
          sub: `Lv.${m.lv}${m.boss ? ' Boss' : ''}`,
          dist: Math.round(Math.hypot(m.x - bx, m.z - bz)),
        });
      }
    }

    results.sort((a, b) => (a.dist || 0) - (b.dist || 0));
    return results.slice(0, 15);
  }, [searchQuery, bot?.stats.pos_x, bot?.stats.pos_z]);

  if (!bot || bot.status !== 'online') {
    return null;
  }

  return (
    <div
      className="pg on"
      style={{
        padding: 0,
        margin: 0,
        height: '100%',
        width: '100%',
        position: 'relative',
        overflow: 'hidden',
        display: 'flex',
      }}
    >
      {/* 100% Full Edge-to-Edge Responsive Map Canvas */}
      <div
        ref={containerRef}
        style={{
          flex: 1,
          width: '100%',
          height: '100%',
          position: 'relative',
          overflow: 'hidden',
          background: '#090304',
        }}
      >
        <canvas
          ref={canvasRef}
          width={canvasSize.width * (window.devicePixelRatio || 1)}
          height={canvasSize.height * (window.devicePixelRatio || 1)}
          style={{
            width: '100%',
            height: '100%',
            display: 'block',
            cursor: isDragging ? 'grabbing' : 'crosshair',
          }}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseLeave}
          onWheel={handleWheel}
          onContextMenu={(e) => e.preventDefault()}
        />

        {/* FLOATING TOP-RIGHT: Toolbar & Search Toggle */}
        <div
          style={{
            position: 'absolute',
            top: '10px',
            right: '10px',
            display: 'flex',
            gap: '6px',
            zIndex: 10,
          }}
        >
          <button
            className="btn btn-secondary"
            style={{ padding: '4px 8px', fontSize: '11px', fontWeight: 'bold' }}
            title="Zoom In"
            onClick={() => setZoom((z) => Math.min(6.0, z * 1.3))}
          >
            +
          </button>
          <button
            className="btn btn-secondary"
            style={{ padding: '4px 8px', fontSize: '11px', fontWeight: 'bold' }}
            title="Zoom Out"
            onClick={() => setZoom((z) => Math.max(0.15, z / 1.3))}
          >
            -
          </button>
          <button
            className="btn btn-primary"
            style={{ padding: '4px 10px', fontSize: '11px' }}
            title="Center on Bot"
            onClick={handleCenterOnBot}
          >
            Locate Bot
          </button>
          <button
            className="btn btn-secondary"
            style={{ padding: '4px 10px', fontSize: '11px' }}
            title="Fit Map to Screen"
            onClick={handleResetWorldView}
          >
            Fit Map
          </button>
          <button
            className={`btn ${showSearchMenu ? 'btn-primary' : 'btn-secondary'}`}
            style={{ padding: '4px 10px', fontSize: '11px', display: 'flex', alignItems: 'center', gap: '4px' }}
            title="Toggle Search & Layers"
            onClick={() => setShowSearchMenu((s) => !s)}
          >
            <Icon name="map" size={14} />
            Search & Layers
          </button>
        </div>

        {/* FLOATING DROPDOWN: Search & Layers Popover */}
        {showSearchMenu && (
          <div
            style={{
              position: 'absolute',
              top: '46px',
              right: '10px',
              width: '290px',
              background: 'rgba(16, 6, 8, 0.94)',
              border: '1px solid var(--ln)',
              borderRadius: '6px',
              boxShadow: '0 8px 24px rgba(0, 0, 0, 0.7)',
              backdropFilter: 'blur(8px)',
              padding: '10px',
              display: 'flex',
              flexDirection: 'column',
              gap: '8px',
              zIndex: 20,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: '11px', fontWeight: 700, color: '#fff' }}>Search & Map Layers</span>
              <button
                className="btn btn-secondary btn-icon"
                style={{ padding: '2px 4px', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
                onClick={() => setShowSearchMenu(false)}
              >
                <Icon name="close" size={14} label="Close" />
              </button>
            </div>

            {/* Search Input */}
            <input
              type="text"
              placeholder="Search NPC, mob, town..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              style={{
                width: '100%',
                padding: '5px 8px',
                background: 'var(--p2)',
                border: '1px solid var(--ln)',
                borderRadius: '4px',
                color: 'var(--t)',
                fontSize: '11px',
              }}
              autoFocus
            />

            {/* Search Hits */}
            {searchResults.length > 0 && (
              <div
                style={{
                  maxHeight: '130px',
                  overflowY: 'auto',
                  border: '1px solid var(--ln)',
                  borderRadius: '3px',
                  background: 'var(--p)',
                }}
              >
                {searchResults.map((item, idx) => (
                  <div
                    key={`${item.name}-${idx}`}
                    style={{
                      padding: '4px 8px',
                      fontSize: '11px',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      borderBottom: '1px solid var(--ln)',
                      cursor: 'pointer',
                    }}
                    onClick={() => {
                      handleJumpToCoords(item.x, item.z);
                      setShowSearchMenu(false);
                    }}
                    title={`Jump to ${item.name}`}
                  >
                    <div>
                      <span
                        style={{
                          color:
                            item.type === 'Shop'
                              ? '#ffd84d'
                              : item.type === 'Mob'
                              ? '#ffe066'
                              : '#6fb6ff',
                        }}
                      >
                        {item.name}
                      </span>
                      <span style={{ marginLeft: '5px', fontSize: '10px', color: 'var(--m)' }}>
                        ({item.sub})
                      </span>
                    </div>
                    <span style={{ fontSize: '10px', color: 'var(--sw)' }}>{item.dist}m</span>
                  </div>
                ))}
              </div>
            )}

            {/* Layer Toggles */}
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: '5px',
                fontSize: '11px',
                paddingTop: '4px',
                borderTop: '1px solid var(--ln)',
              }}
            >
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.monsters}
                  onChange={(e) => setLayers((l) => ({ ...l, monsters: e.target.checked }))}
                />
                Monsters
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.npcs}
                  onChange={(e) => setLayers((l) => ({ ...l, npcs: e.target.checked }))}
                />
                NPCs & Shops
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.towns}
                  onChange={(e) => setLayers((l) => ({ ...l, towns: e.target.checked }))}
                />
                Towns & Places
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.lodestars}
                  onChange={(e) => setLayers((l) => ({ ...l, lodestars: e.target.checked }))}
                />
                Lodestars
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.otherBots}
                  onChange={(e) => setLayers((l) => ({ ...l, otherBots: e.target.checked }))}
                />
                Other Bots
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={layers.labels}
                  onChange={(e) => setLayers((l) => ({ ...l, labels: e.target.checked }))}
                />
                Labels
              </label>
            </div>
          </div>
        )}

        {/* FLOATING BOTTOM-LEFT: Cursor & Instructions */}
        <div
          style={{
            position: 'absolute',
            bottom: '10px',
            left: '10px',
            background: 'rgba(12, 3, 4, 0.85)',
            border: '1px solid var(--ln)',
            borderRadius: '4px',
            padding: '4px 10px',
            fontSize: '11px',
            color: 'var(--m)',
            display: 'flex',
            gap: '12px',
            pointerEvents: 'none',
            zIndex: 10,
          }}
        >
          <span>
            <b style={{ color: 'var(--t)' }}>{cursorPos ? `X ${cursorPos.x} Z ${cursorPos.z}` : '—'}</b>
          </span>
        </div>

        {/* FLOATING BOTTOM-RIGHT: Legend */}
        <div
          style={{
            position: 'absolute',
            bottom: '10px',
            right: '10px',
            background: 'rgba(12, 3, 4, 0.85)',
            border: '1px solid var(--ln)',
            borderRadius: '4px',
            padding: '4px 8px',
            fontSize: '10px',
            display: 'flex',
            gap: '8px',
            alignItems: 'center',
            pointerEvents: 'none',
            zIndex: 10,
          }}
        >
          <span style={{ color: '#ffd84d' }}>● You</span>
          <span style={{ color: '#3ddc97' }}>● Party</span>
          <span style={{ color: '#6fb6ff' }}>● NPC</span>
          <span style={{ color: '#ffd84d' }}>● Shop</span>
          <span style={{ color: '#fff' }}>+ Lodestar</span>
        </div>
      </div>
    </div>
  );
};

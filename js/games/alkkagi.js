  // ── Alkkagi UI (Matter.js 물리 보드, 슬링샷 방식) ─────────────────────────────

  let alkkagiEngine = null;
  let alkkagiRender = null;
  let alkkagiRunner = null;
  let alkkagiWorld = null;
  let alkkagiBodies = {};  // id -> Matter.Body
  let alkkagiInitialized = false;
  let alkkagiPlacementCanvas = null;
  let alkkagiPhase = 'ready';

  let alkkagiMyColor = 0;
  let alkkagiMyTurn = false;
  window.alkkagiJustFlicked = false;

  const ALKKAGI_W = 420, ALKKAGI_H = 420;
  const ALKKAGI_CELL = 28;
  const ALKKAGI_GRID = 15;

  const ALKKAGI_JANGGI = {
    K: { maxPower: 0.084, mass: 4.0, radius: 24, char: '將' },
    R: { maxPower: 0.078, mass: 2.5, radius: 20, char: '車' },
    P: { maxPower: 0.072, mass: 2.0, radius: 19, char: '包' },
    H: { maxPower: 0.066, mass: 1.6, radius: 18, char: '馬' },
    E: { maxPower: 0.066, mass: 1.6, radius: 18, char: '象' },
    S: { maxPower: 0.06, mass: 1.2, radius: 15, char: '卒' }
  };
  const ALKKAGI_CHESS = {
    K: { maxPower: 0.084, mass: 4.0, radius: 24, char: '♔' },
    Q: { maxPower: 0.078, mass: 3.0, radius: 22, char: '♕' },
    R: { maxPower: 0.078, mass: 2.5, radius: 20, char: '♖' },
    B: { maxPower: 0.066, mass: 1.8, radius: 18, char: '♗' },
    N: { maxPower: 0.066, mass: 1.6, radius: 17, char: '♘' },
    P: { maxPower: 0.06, mass: 1.2, radius: 15, char: '♙' }
  };
  const ALKKAGI_ORIGINAL = { _: { maxPower: 0.06, mass: 1.1, radius: 18, char: '' } };

  let alkkagiSelectedStone = null;
  let alkkagiValidGuides = [];
  let alkkagiStonesData = [];
  let alkkagiMode = 'janggi';
  let alkkagiPlacementMousePos = null;
  let alkkagiPlacementNextRole = '';
  let alkkagiPlacementData = null;
  let alkkagiSlingshotBody = null;
  let alkkagiSlingshotEnd = null;

  function getAlkkagiCfg(role, mode) {
    if (mode === 'chess') return ALKKAGI_CHESS[role] || ALKKAGI_CHESS.N;
    if (mode === 'janggi') return ALKKAGI_JANGGI[role] || ALKKAGI_JANGGI.E;
    return ALKKAGI_ORIGINAL._;
  }

  function getAlkkagiRadius(role, mode) {
    const cfg = getAlkkagiCfg(role, mode);
    return (cfg && cfg.radius != null) ? cfg.radius : 18;
  }

  function getAlkkagiChar(role, mode) {
    const cfg = getAlkkagiCfg(role, mode);
    return cfg.char || '';
  }

  function pxToCell(x, y) {
    return {
      col: Math.floor(x / ALKKAGI_CELL),
      row: Math.floor(y / ALKKAGI_CELL)
    };
  }

  function cellToPx(col, row) {
    return {
      x: (col + 0.5) * ALKKAGI_CELL,
      y: (row + 0.5) * ALKKAGI_CELL
    };
  }

  function getValidChessMoves(stone, allStones) {
    const role = (stone.role || '').toUpperCase();
    const occupied = {};
    allStones.forEach(s => {
      const c = pxToCell(s.x, s.y);
      occupied[`${c.col},${c.row}`] = s.color;
    });
    const sc = pxToCell(stone.x, stone.y);
    const col = sc.col;
    const row = sc.row;
    const color = stone.color;
    const moves = [];

    const inBounds = (c, r) => c >= 0 && c < ALKKAGI_GRID && r >= 0 && r < ALKKAGI_GRID;
    const isFriendly = (c, r) => occupied[`${c},${r}`] === color;
    const isEnemy = (c, r) => {
      const oc = occupied[`${c},${r}`];
      return oc && oc !== color;
    };
    const addIfValid = (c, r, stopAtPiece) => {
      if (!inBounds(c, r)) return false;
      if (isFriendly(c, r)) return true;
      moves.push({ col: c, row: r });
      if (isEnemy(c, r) || stopAtPiece) return true;
      return false;
    };

    if (!role || alkkagiMode === 'original') {
      for (let dc = -1; dc <= 1; dc++) {
        for (let dr = -1; dr <= 1; dr++) {
          if (dc === 0 && dr === 0) continue;
          addIfValid(col + dc, row + dr, true);
        }
      }
      return moves;
    }

    if (role === 'K') {
      for (let dc = -1; dc <= 1; dc++) {
        for (let dr = -1; dr <= 1; dr++) {
          if (dc === 0 && dr === 0) continue;
          addIfValid(col + dc, row + dr, true);
        }
      }
    } else if (role === 'Q') {
      for (const [dc, dr] of [[1,0],[-1,0],[0,1],[0,-1],[1,1],[1,-1],[-1,1],[-1,-1]]) {
        for (let d = 1; d < ALKKAGI_GRID; d++) {
          if (addIfValid(col + dc * d, row + dr * d, true)) break;
        }
      }
    } else if (role === 'R' || role === 'P') {
      for (const [dc, dr] of [[1,0],[-1,0],[0,1],[0,-1]]) {
        for (let d = 1; d < ALKKAGI_GRID; d++) {
          if (addIfValid(col + dc * d, row + dr * d, true)) break;
        }
      }
    } else if (role === 'B' || role === 'E') {
      for (const [dc, dr] of [[1,1],[1,-1],[-1,1],[-1,-1]]) {
        for (let d = 1; d < ALKKAGI_GRID; d++) {
          if (addIfValid(col + dc * d, row + dr * d, true)) break;
        }
      }
    } else if (role === 'N' || role === 'H') {
      const jumps = [[2,1],[2,-1],[-2,1],[-2,-1],[1,2],[1,-2],[-1,2],[-1,-2]];
      jumps.forEach(([dc, dr]) => addIfValid(col + dc, row + dr, true));
    } else if (role === 'S') {
      for (let dc = -1; dc <= 1; dc++) {
        for (let dr = -1; dr <= 1; dr++) {
          if (dc === 0 && dr === 0) continue;
          addIfValid(col + dc, row + dr, true);
        }
      }
    } else {
      for (const [dc, dr] of [[1,0],[-1,0],[0,1],[0,-1]]) {
        for (let d = 1; d < ALKKAGI_GRID; d++) {
          if (addIfValid(col + dc * d, row + dr * d, true)) break;
        }
      }
    }
    return moves;
  }

  function showAlkkagiUI() {
    switchGameView('alkkagi');
  }

  function renderAlkkagi(data) {
    if (!data) return;
    alkkagiMode = data.mode || 'janggi';
    alkkagiPhase = data.phase || 'ready';
    if (alkkagiPhase === 'ready') {
      if (typeof window.clearAlkkagi === 'function') window.clearAlkkagi();
      const readyArea = document.getElementById('ready-area');
      const rematchArea = document.getElementById('rematch-area');
      if (readyArea) { readyArea.style.display = 'flex'; }
      if (rematchArea) { rematchArea.style.display = 'none'; }
    }
    const players = data.players || [];
    if (players[0] === currentUserId) alkkagiMyColor = 1;
    else if (players[1] === currentUserId) alkkagiMyColor = 2;
    else alkkagiMyColor = 0;
    const prevMyTurn = alkkagiMyTurn;
    alkkagiMyTurn = (data.currentTurn === currentUserId);
    if (prevMyTurn !== alkkagiMyTurn) {
      alkkagiSelectedStone = null;
      alkkagiSlingshotBody = null;
      alkkagiSlingshotEnd = null;
      alkkagiValidGuides = [];
    }

    const statusEl = document.getElementById('alkkagi-status');
    const phaseEl = document.getElementById('alkkagi-phase');
    const stonesEl = document.getElementById('alkkagi-stones-count');
    const nextRoleEl = document.getElementById('alkkagi-next-role');
    if (nextRoleEl) {
      const nextRole = alkkagiMyColor === 1 ? (data.nextRoleBlack || '') : (alkkagiMyColor === 2 ? (data.nextRoleWhite || '') : '');
      if (alkkagiPhase === 'placement' && nextRole) {
        const char = getAlkkagiChar(nextRole, alkkagiMode);
        const iconSize = alkkagiMode === 'chess' ? '1.4em' : '1em';
        nextRoleEl.innerHTML = `다음 배치: <span class="alkkagi-next-role-icon" style="font-size:${iconSize};font-weight:800;">${char || '●'}</span>`;
        nextRoleEl.style.display = 'inline';
      } else {
        nextRoleEl.style.display = 'none';
      }
    }
    if (statusEl) {
      if (alkkagiPhase === 'placement') {
        const nextRole = alkkagiMyColor === 1 ? (data.nextRoleBlack || '') : (alkkagiMyColor === 2 ? (data.nextRoleWhite || '') : '');
        const nextChar = nextRole ? getAlkkagiChar(nextRole, alkkagiMode) || '●' : '';
        statusEl.textContent = nextChar ? `배치 중 — 다음: ${nextChar} — 자기 영역의 격자를 클릭하여 돌을 놓으세요` : '배치 중 — 자기 영역의 격자를 클릭하여 돌을 놓으세요';
      } else if (alkkagiPhase === 'playing') {
        const turn = data.currentTurn || '';
        statusEl.textContent = turn === currentUserId
          ? '🎯 내 차례 — 돌을 클릭하고 쏠 방향을 조절하세요'
          : turn ? `⏳ ${escapeHTML(turn)}의 차례` : '알까기';
      } else {
        statusEl.textContent = '알까기 — 상대방을 기다리는 중...';
      }
    }
    if (phaseEl) {
      phaseEl.textContent = alkkagiPhase === 'placement' ? '배치 중' : alkkagiPhase === 'playing' ? '대전 중' : '준비';
    }
    if (stonesEl) {
      const stones = data.stones || [];
      const teams = data.teams || ['한', '초'];
      const c1 = stones.filter(s => s.color === 1).length;
      const c2 = stones.filter(s => s.color === 2).length;
      stonesEl.textContent = stones.length ? `${teams[0] || '한'} ${c1} : ${teams[1] || '초'} ${c2}` : `${teams[0] || '한'} 7 : ${teams[1] || '초'} 7`;
    }
    const timerEl = document.getElementById('alkkagi-placement-timer');
    if (timerEl) {
      const rem = data.placementRemaining || 0;
      if (alkkagiPhase === 'placement' && rem > 0) {
        timerEl.style.display = 'inline';
        timerEl.textContent = `⏱ ${rem}초`;
      } else {
        timerEl.style.display = 'none';
      }
    }

    const wrap = document.getElementById('alkkagi-board-wrap');
    if (!wrap || typeof Matter === 'undefined') return;

    if (alkkagiPhase === 'placement') {
      alkkagiPlacementData = data;
      if (!alkkagiPlacementCanvas) {
        initAlkkagiPlacement(wrap, data);
      } else {
        updateAlkkagiPlacement(data);
      }
    } else if (alkkagiPhase === 'playing') {
      if (alkkagiPlacementCanvas) {
        alkkagiPlacementCanvas.remove();
        alkkagiPlacementCanvas = null;
      }
      if (!alkkagiInitialized) {
        alkkagiInitialized = true;
        initAlkkagiPhysics(wrap, data);
      } else {
        syncAlkkagiStones(data.stones || []);
      }
    } else {
      if (alkkagiPlacementCanvas) {
        alkkagiPlacementCanvas.remove();
        alkkagiPlacementCanvas = null;
      }
    }
  }

  function initAlkkagiPlacement(container, data) {
    const canvas = document.createElement('canvas');
    canvas.width = ALKKAGI_W;
    canvas.height = ALKKAGI_H;
    canvas.style.cssText = 'display:block; cursor:pointer; border-radius:8px; touch-action: none;';
    container.appendChild(canvas);
    alkkagiPlacementCanvas = canvas;
    alkkagiPlacementData = data;
    alkkagiPlacementNextRole = alkkagiMyColor === 1 ? (data.nextRoleBlack || '') : (alkkagiMyColor === 2 ? (data.nextRoleWhite || '') : '');
    updateAlkkagiPlacement(data);

    function onPlacementPointerMove(e) {
      const rect = canvas.getBoundingClientRect();
      const scaleX = canvas.width / rect.width;
      const scaleY = canvas.height / rect.height;
      const clientX = e.clientX ?? (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      const clientY = e.clientY ?? (e.touches && e.touches[0] ? e.touches[0].clientY : 0);
      alkkagiPlacementMousePos = { x: (clientX - rect.left) * scaleX, y: (clientY - rect.top) * scaleY };
      updateAlkkagiPlacement(alkkagiPlacementData);
    }
    function onPlacementPointerLeave() {
      alkkagiPlacementMousePos = null;
      updateAlkkagiPlacement(alkkagiPlacementData);
    }
    canvas.addEventListener('mousemove', onPlacementPointerMove);
    canvas.addEventListener('mouseleave', onPlacementPointerLeave);
    canvas.addEventListener('pointermove', onPlacementPointerMove);
    canvas.addEventListener('pointerleave', onPlacementPointerLeave);

    canvas.addEventListener('click', function(e) {
      if (alkkagiMyColor === 0 || alkkagiPhase !== 'placement') return;
      const rect = canvas.getBoundingClientRect();
      const scaleX = canvas.width / rect.width;
      const scaleY = canvas.height / rect.height;
      const x = (e.clientX - rect.left) * scaleX;
      const y = (e.clientY - rect.top) * scaleY;
      const col = Math.floor(x / ALKKAGI_CELL);
      const row = Math.floor(y / ALKKAGI_CELL);
      if (col < 0 || col >= 15 || row < 0 || row >= 15) return;
      if (alkkagiMyColor === 1 && (row < 10 || row > 14)) return;
      if (alkkagiMyColor === 2 && (row < 0 || row > 4)) return;
      if (typeof sendGameAction === 'function') {
        sendGameAction({ cmd: 'place', col, row });
      }
    });
  }

  function updateAlkkagiPlacement(data) {
    if (!alkkagiPlacementCanvas) return;
    alkkagiPlacementNextRole = alkkagiMyColor === 1 ? (data && data.nextRoleBlack) || '' : (alkkagiMyColor === 2 ? (data && data.nextRoleWhite) || '' : '');
    const mode = (data && data.mode) || 'janggi';
    const ctx = alkkagiPlacementCanvas.getContext('2d');
    ctx.fillStyle = '#c8a45a';
    ctx.fillRect(0, 0, ALKKAGI_W, ALKKAGI_H);
    if (alkkagiPhase === 'placement' && alkkagiMyColor > 0) {
      ctx.fillStyle = alkkagiMyColor === 1 ? 'rgba(220,38,38,0.18)' : 'rgba(59,130,246,0.18)';
      ctx.fillRect(0, alkkagiMyColor === 1 ? 10 * ALKKAGI_CELL : 0, ALKKAGI_W, 5 * ALKKAGI_CELL);
    }
    ctx.strokeStyle = 'rgba(122,80,16,0.5)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 15; i++) {
      ctx.beginPath();
      ctx.moveTo(i * ALKKAGI_CELL, 0);
      ctx.lineTo(i * ALKKAGI_CELL, ALKKAGI_H);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(0, i * ALKKAGI_CELL);
      ctx.lineTo(ALKKAGI_W, i * ALKKAGI_CELL);
      ctx.stroke();
    }
    const stones = (data && data.stones) || [];
    stones.forEach(s => {
      const role = (s.role || '').toUpperCase();
      const char = getAlkkagiChar(role, mode);
      const radius = getAlkkagiRadius(role, mode);
      if (mode === 'original') {
        ctx.fillStyle = s.color === 1 ? '#1a1a2e' : '#fff5f5';
        ctx.strokeStyle = s.color === 1 ? '#334155' : '#94a3b8';
      } else {
        ctx.fillStyle = s.color === 1 ? '#fff5f5' : '#1a1a2e';
        ctx.strokeStyle = s.color === 1 ? '#dc2626' : '#22c55e';
      }
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(s.x, s.y, radius, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
      const fontSz = Math.max(14, Math.min(22, radius * 1.1));
      ctx.font = `bold ${fontSz}px "Noto Sans KR", "Malgun Gothic", sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillStyle = (mode === 'original' && !char) ? (s.color === 1 ? '#94a3b8' : '#64748b') : (s.color === 1 ? '#dc2626' : '#3b82f6');
      if (char) ctx.fillText(char, s.x, s.y);
      else if (mode === 'original') ctx.fillText('●', s.x, s.y);
    });
    if (alkkagiPlacementMousePos && alkkagiMyColor > 0 && alkkagiPlacementNextRole !== undefined) {
      const col = Math.floor(alkkagiPlacementMousePos.x / ALKKAGI_CELL);
      const row = Math.floor(alkkagiPlacementMousePos.y / ALKKAGI_CELL);
      const inBounds = col >= 0 && col < 15 && row >= 0 && row < 15;
      const inMyZone = (alkkagiMyColor === 1 && row >= 10 && row <= 14) || (alkkagiMyColor === 2 && row >= 0 && row <= 4);
      const occupied = stones.some(s => {
        const sc = Math.floor(s.x / ALKKAGI_CELL);
        const sr = Math.floor(s.y / ALKKAGI_CELL);
        return sc === col && sr === row;
      });
      if (inBounds && inMyZone && !occupied) {
        const gx = (col + 0.5) * ALKKAGI_CELL;
        const gy = (row + 0.5) * ALKKAGI_CELL;
        const ghostChar = getAlkkagiChar(alkkagiPlacementNextRole, mode);
        const ghostRadius = getAlkkagiRadius(alkkagiPlacementNextRole, mode);
        ctx.globalAlpha = 0.75;
        if (mode === 'original') {
          ctx.fillStyle = alkkagiMyColor === 1 ? '#1a1a2e' : '#fff5f5';
          ctx.strokeStyle = alkkagiMyColor === 1 ? '#334155' : '#94a3b8';
        } else {
          ctx.fillStyle = alkkagiMyColor === 1 ? '#fff5f5' : '#1a1a2e';
          ctx.strokeStyle = alkkagiMyColor === 1 ? '#dc2626' : '#3b82f6';
        }
        ctx.lineWidth = 5;
        ctx.beginPath();
        ctx.arc(gx, gy, ghostRadius, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
        const displayChar = ghostChar || '●';
        const ghostFontSz = Math.max(22, Math.min(32, ghostRadius * 1.5));
        ctx.font = `bold ${ghostFontSz}px "Noto Sans KR", "Malgun Gothic", sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = (mode === 'original') ? (alkkagiMyColor === 1 ? '#94a3b8' : '#64748b') : (alkkagiMyColor === 1 ? '#dc2626' : '#3b82f6');
        ctx.fillText(displayChar, gx, gy);
        ctx.font = 'bold 16px "Noto Sans KR", sans-serif';
        ctx.fillStyle = 'rgba(0,0,0,0.85)';
        ctx.fillText('⌖ 배치 예정', gx, gy + ghostRadius + 16);
        ctx.globalAlpha = 1;
      }
    }
  }

  function initAlkkagiPhysics(container, data) {
    const M = Matter;
    const W = ALKKAGI_W, H = ALKKAGI_H;

    const engine = M.Engine.create();
    engine.gravity.x = 0;
    engine.gravity.y = 0;

    const world = engine.world;

    const stones = data.stones || [];
    alkkagiMode = data.mode || 'janggi';
    alkkagiStonesData = stones.map(s => ({ ...s }));
    stones.forEach(s => {
      const role = (s.role || '').toUpperCase();
      const cfg = getAlkkagiCfg(role, alkkagiMode);
      const mass = cfg.mass;
      const radius = getAlkkagiRadius(role, alkkagiMode);
      let fill, stroke;
      if (alkkagiMode === 'original') {
        fill = s.color === 1 ? '#1a1a2e' : '#fff5f5';
        stroke = s.color === 1 ? '#334155' : '#94a3b8';
      } else {
        fill = s.color === 1 ? '#fff5f5' : '#1a1a2e';
        stroke = s.color === 1 ? '#dc2626' : '#22c55e';
      }
      const body = M.Bodies.circle(s.x || 100, s.y || 100, radius, {
        friction: 0.05, frictionAir: 0.02, restitution: 0.5,
        render: { fillStyle: fill, strokeStyle: stroke, lineWidth: 2 },
      });
      M.Body.setMass(body, mass);
      body.alkkagiId = s.id;
      body.alkkagiColor = s.color;
      body.alkkagiRole = role;
      alkkagiBodies[s.id] = body;
      M.World.add(world, body);
    });

    const render = M.Render.create({
      element: container,
      engine: engine,
      options: {
        width: W,
        height: H,
        wireframes: false,
        background: '#c8a45a',
      },
    });
    M.Render.run(render);

    M.Events.on(render, 'beforeRender', function() {
      const ctx = render.context;
      ctx.strokeStyle = 'rgba(122,80,16,0.7)';
      ctx.lineWidth = 1.2;
      const cellW = W / 15, cellH = H / 15;
      for (let i = 0; i <= 15; i++) {
        ctx.beginPath();
        ctx.moveTo(i * cellW, 0);
        ctx.lineTo(i * cellW, H);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(0, i * cellH);
        ctx.lineTo(W, i * cellH);
        ctx.stroke();
      }
    });

    M.Events.on(render, 'afterRender', function() {
      const ctx = render.context;
      Object.keys(alkkagiBodies).forEach(id => {
        const b = alkkagiBodies[id];
        if (!b || !world.bodies.includes(b)) return;
        const role = (b.alkkagiRole || '').toUpperCase();
        const char = getAlkkagiChar(role, alkkagiMode);
        const displayChar = char || (alkkagiMode === 'original' ? '●' : '');
        ctx.save();
        ctx.translate(b.position.x, b.position.y);
        ctx.rotate(b.angle);
        const radius = getAlkkagiRadius(role, alkkagiMode);
        const fontSz = Math.max(14, Math.min(22, radius * 1.1));
        ctx.font = `bold ${fontSz}px "Noto Sans KR", "Malgun Gothic", sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = (alkkagiMode === 'original') ? (b.alkkagiColor === 1 ? '#94a3b8' : '#64748b') : (b.alkkagiColor === 1 ? '#dc2626' : '#3b82f6');
        if (displayChar) ctx.fillText(displayChar, 0, 0);
        ctx.restore();
      });
      if (alkkagiMode === 'chess' && alkkagiSelectedStone && alkkagiValidGuides.length > 0) {
        ctx.fillStyle = 'rgba(255,255,255,0.5)';
        ctx.strokeStyle = 'rgba(255,255,255,0.8)';
        ctx.lineWidth = 2;
        alkkagiValidGuides.forEach(g => {
          const p = cellToPx(g.col, g.row);
          ctx.beginPath();
          ctx.arc(p.x, p.y, 14, 0, Math.PI * 2);
          ctx.fill();
          ctx.stroke();
        });
      }
      if (alkkagiSlingshotBody && alkkagiSlingshotEnd) {
        const b = alkkagiSlingshotBody;
        const end = alkkagiSlingshotEnd;
        const dx = end.x - b.position.x;
        const dy = end.y - b.position.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const ux = dx / dist;
        const uy = dy / dist;
        ctx.strokeStyle = 'rgba(255,255,255,0.9)';
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.moveTo(b.position.x, b.position.y);
        ctx.lineTo(end.x, end.y);
        ctx.stroke();
        const numDots = 5;
        const step = dist / (numDots + 1);
        for (let i = 1; i <= numDots; i++) {
          const px = b.position.x + ux * step * i;
          const py = b.position.y + uy * step * i;
          const r = Math.max(3, 6 - i * 0.8);
          ctx.fillStyle = 'rgba(255,255,255,0.85)';
          ctx.strokeStyle = 'rgba(88,166,255,0.9)';
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.arc(px, py, r, 0, Math.PI * 2);
          ctx.fill();
          ctx.stroke();
        }
      }
    });

    const runner = M.Runner.create();
    M.Runner.run(runner, engine);

    let alkkagiWasMoving = false;

    M.Events.on(engine, 'afterUpdate', function() {
      const bodies = Object.values(alkkagiBodies);
      let isMoving = false;
      bodies.forEach(b => {
        if (!b || !world.bodies.includes(b)) return;
        const v = b.velocity;
        const speed = Math.sqrt(v.x * v.x + v.y * v.y);
        if (speed > 0.1) isMoving = true;
        const x = b.position.x, y = b.position.y;
        if (x < -50 || x > 470 || y < -50 || y > 470) {
          if (window.SoundManager) window.SoundManager.playPianoNote(130.81, 0.25);
          M.Composite.remove(world, b);
          delete alkkagiBodies[b.alkkagiId];
        }
      });

      if (alkkagiWasMoving && !isMoving && window.alkkagiJustFlicked) {
        window.alkkagiJustFlicked = false;
        const currentStones = [];
        Object.keys(alkkagiBodies).forEach(id => {
          const b = alkkagiBodies[id];
          if (b && world.bodies.includes(b)) {
            currentStones.push({
              id: b.alkkagiId,
              x: b.position.x,
              y: b.position.y,
              velX: b.velocity.x,
              velY: b.velocity.y,
              color: b.alkkagiColor,
              role: b.alkkagiRole || 'P',
            });
          }
        });
        if (typeof sendGameAction === 'function') {
          sendGameAction({ cmd: 'sync', stones: currentStones });
        }
      }
      alkkagiWasMoving = isMoving;
    });

    M.Events.on(engine, 'collisionStart', function(ev) {
      ev.pairs.forEach(pair => {
        const speed = Math.sqrt(
          (pair.bodyA.velocity.x - pair.bodyB.velocity.x) ** 2 +
          (pair.bodyA.velocity.y - pair.bodyB.velocity.y) ** 2
        );
        if (speed > 0.5 && window.SoundManager) {
          const massA = pair.bodyA.mass || 1;
          const massB = pair.bodyB.mass || 1;
          const avgMass = (massA + massB) / 2;
          const freq = avgMass >= 2.5 ? 130.81 : (avgMass >= 1.5 ? 196 : 523.25);
          window.SoundManager.playPianoNote(freq, 0.15);
        }
      });
    });

    function canvasToWorld(e) {
      const canvas = render.canvas;
      const rect = canvas.getBoundingClientRect();
      const scaleX = canvas.width / rect.width;
      const scaleY = canvas.height / rect.height;
      return {
        x: (e.clientX - rect.left) * scaleX,
        y: (e.clientY - rect.top) * scaleY,
      };
    }

    function allStonesStopped() {
      const bodies = Object.values(alkkagiBodies);
      for (let i = 0; i < bodies.length; i++) {
        const b = bodies[i];
        if (!b || !world.bodies.includes(b)) continue;
        const v = b.velocity;
        const speed = Math.sqrt(v.x * v.x + v.y * v.y);
        if (speed > 0.1) return false;
      }
      return true;
    }

    function getStonesForMoves() {
      const list = [];
      Object.keys(alkkagiBodies).forEach(id => {
        const b = alkkagiBodies[id];
        if (b && world.bodies.includes(b)) {
          list.push({ x: b.position.x, y: b.position.y, color: b.alkkagiColor });
        }
      });
      return list;
    }

    function hitTestGuide(pos) {
      for (let i = 0; i < alkkagiValidGuides.length; i++) {
        const g = alkkagiValidGuides[i];
        const p = cellToPx(g.col, g.row);
        const dx = pos.x - p.x;
        const dy = pos.y - p.y;
        if (dx * dx + dy * dy <= 20 * 20) return i;
      }
      return -1;
    }

    function applySlingshotFlick(body, startPos, endPos, towardTarget) {
      const dx = towardTarget ? (endPos.x - startPos.x) : (startPos.x - endPos.x);
      const dy = towardTarget ? (endPos.y - startPos.y) : (startPos.y - endPos.y);
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const cellDist = dist / ALKKAGI_CELL;
      const role = (body.alkkagiRole || '').toUpperCase();
      const cfg = getAlkkagiCfg(role, alkkagiMode);
      const massFactor = cfg.mass;
      let forceMag = Math.min(cellDist * 0.012, cfg.maxPower) * massFactor;
      forceMag = Math.min(forceMag, cfg.maxPower * massFactor);
      const ux = dx / dist;
      const uy = dy / dist;
      let fx = ux * forceMag;
      let fy = uy * forceMag;
      if (cellDist >= 3) {
        const varianceDeg = (Math.random() - 0.5) * 6;
        const angle = Math.atan2(fy, fx);
        const newAngle = angle + (varianceDeg * Math.PI / 180);
        const mag = Math.sqrt(fx * fx + fy * fy);
        fx = mag * Math.cos(newAngle);
        fy = mag * Math.sin(newAngle);
      }
      if (window.SoundManager) window.SoundManager.playPianoNote(98, 0.2);
      if (typeof sendGameAction === 'function') {
        sendGameAction({ cmd: 'flick', id: body.alkkagiId, forceX: fx, forceY: fy });
      }
      window.alkkagiJustFlicked = true;
    }

    render.canvas.addEventListener('pointerdown', function(e) {
      e.preventDefault();
      render.canvas.setPointerCapture(e.pointerId);
      const pos = canvasToWorld(e);
      if (!alkkagiMyTurn || alkkagiMyColor === 0 || !allStonesStopped()) return;

      if (alkkagiMode === 'chess') {
        const guideIdx = hitTestGuide(pos);
        if (guideIdx >= 0 && alkkagiSelectedStone) {
          const body = alkkagiSelectedStone;
          const g = alkkagiValidGuides[guideIdx];
          const targetPx = cellToPx(g.col, g.row);
          applySlingshotFlick(body, body.position, targetPx, true);
          alkkagiSelectedStone = null;
          alkkagiValidGuides = [];
          return;
        }
      } else {
        if (alkkagiSlingshotBody) {
          alkkagiSlingshotBody = null;
          alkkagiSlingshotEnd = null;
        }
      }

      const bodies = Object.values(alkkagiBodies).filter(b => b && b.alkkagiColor === alkkagiMyColor);
      const hit = M.Query.point(bodies, pos);
      if (hit.length > 0) {
        const body = hit[0];
        if (alkkagiMode === 'chess') {
          const stoneData = { x: body.position.x, y: body.position.y, color: body.alkkagiColor, role: body.alkkagiRole };
          alkkagiValidGuides = getValidChessMoves(stoneData, getStonesForMoves());
          alkkagiSelectedStone = body;
          if (window.SoundManager) {
            window.SoundManager.playPianoNote(523.25, 0.15);
            setTimeout(() => { if (window.SoundManager) window.SoundManager.playPianoNote(659.25, 0.15); }, 80);
          }
        } else {
          alkkagiSlingshotBody = body;
          alkkagiSlingshotEnd = { x: pos.x, y: pos.y };
        }
      } else {
        alkkagiSelectedStone = null;
        alkkagiValidGuides = [];
      }
    });

    render.canvas.addEventListener('pointermove', function(e) {
      if (alkkagiSlingshotBody) {
        e.preventDefault();
        const pos = canvasToWorld(e);
        alkkagiSlingshotEnd = { x: pos.x, y: pos.y };
      }
    });

    render.canvas.addEventListener('pointerup', function(e) {
      if (alkkagiSlingshotBody) {
        e.preventDefault();
        const pos = canvasToWorld(e);
        const body = alkkagiSlingshotBody;
        const startPos = { x: body.position.x, y: body.position.y };
        const dist = Math.sqrt((pos.x - startPos.x) ** 2 + (pos.y - startPos.y) ** 2);
        if (dist >= 10) {
          applySlingshotFlick(body, startPos, pos, false);
        }
        alkkagiSlingshotBody = null;
        alkkagiSlingshotEnd = null;
      }
    });

    render.canvas.addEventListener('pointercancel', function() {
      alkkagiSlingshotBody = null;
      alkkagiSlingshotEnd = null;
    });

    render.canvas.style.touchAction = 'none';

    alkkagiEngine = engine;
    alkkagiRender = render;
    alkkagiRunner = runner;
    alkkagiWorld = world;
  }

  function allStonesStoppedForSync() {
    if (!alkkagiWorld || !alkkagiBodies) return true;
    const bodies = Object.values(alkkagiBodies);
    for (let i = 0; i < bodies.length; i++) {
      const b = bodies[i];
      if (!b || !alkkagiWorld.bodies.includes(b)) continue;
      const v = b.velocity;
      const speed = Math.sqrt(v.x * v.x + v.y * v.y);
      if (speed > 0.1) return false;
    }
    return true;
  }

  function syncAlkkagiStones(stones) {
    if (!alkkagiWorld || !alkkagiBodies) return;
    if (window.alkkagiJustFlicked || !allStonesStoppedForSync()) return;
    alkkagiStonesData = stones.map(s => ({ ...s }));
    const ids = new Set(stones.map(s => s.id));
    Object.keys(alkkagiBodies).forEach(id => {
      if (!ids.has(parseInt(id, 10))) {
        const b = alkkagiBodies[id];
        if (b) Matter.Composite.remove(alkkagiWorld, b);
        delete alkkagiBodies[id];
      }
    });
    stones.forEach(s => {
      let body = alkkagiBodies[s.id];
      const role = (s.role || '').toUpperCase();
      const cfg = getAlkkagiCfg(role, alkkagiMode);
      const mass = cfg.mass;
      let fill, stroke;
      if (alkkagiMode === 'original') {
        fill = s.color === 1 ? '#1a1a2e' : '#fff5f5';
        stroke = s.color === 1 ? '#334155' : '#94a3b8';
      } else {
        fill = s.color === 1 ? '#fff5f5' : '#1a1a2e';
        stroke = s.color === 1 ? '#dc2626' : '#22c55e';
      }
      if (body) {
        Matter.Body.setPosition(body, { x: s.x, y: s.y });
        Matter.Body.setVelocity(body, { x: s.velX || 0, y: s.velY || 0 });
        Matter.Body.setMass(body, mass);
        body.alkkagiRole = role;
      } else {
        const M = Matter;
        const radius = getAlkkagiRadius(role, alkkagiMode);
        body = M.Bodies.circle(s.x || 100, s.y || 100, radius, {
          friction: 0.05, frictionAir: 0.02, restitution: 0.5,
          render: { fillStyle: fill, strokeStyle: stroke, lineWidth: 2 },
        });
        M.Body.setMass(body, mass);
        body.alkkagiId = s.id;
        body.alkkagiColor = s.color;
        body.alkkagiRole = role;
        alkkagiBodies[s.id] = body;
        M.World.add(alkkagiWorld, body);
      }
    });
  }

  window.handleAlkkagiFlick = function(data) {
    if (!alkkagiWorld || !alkkagiBodies) return;
    const body = alkkagiBodies[data.id];
    if (!body || !alkkagiWorld.bodies.includes(body)) return;
    let fx = data.forceX || 0;
    let fy = data.forceY || 0;
    const mag = Math.sqrt(fx * fx + fy * fy);
    if (mag > 0.001) {
      const maxMag = 0.08;
      const t = Math.min(1, mag / maxMag);
      const hash = (data.id * 7 + Math.floor(fx * 1000) + Math.floor(fy * 1000) + 12345) % 100;
      const varianceDeg = t * (3 + (hash % 20) / 10) * ((hash % 2 === 0) ? 1 : -1);
      const angle = Math.atan2(fy, fx);
      const newAngle = angle + (varianceDeg * Math.PI / 180);
      fx = mag * Math.cos(newAngle);
      fy = mag * Math.sin(newAngle);
    }
    Matter.Body.applyForce(body, body.position, { x: fx, y: fy });
  };

  window.clearAlkkagi = function() {
    alkkagiSlingshotBody = null;
    alkkagiSlingshotEnd = null;
    alkkagiSelectedStone = null;
    alkkagiValidGuides = [];
    alkkagiPlacementNextRole = '';
    alkkagiPlacementData = null;
    alkkagiPlacementMousePos = null;
    alkkagiStonesData = [];
    alkkagiPhase = 'ready';
    window.alkkagiJustFlicked = false;
    if (alkkagiPlacementCanvas) {
      alkkagiPlacementCanvas.remove();
      alkkagiPlacementCanvas = null;
    }
    if (alkkagiEngine) {
      const M = Matter;
      if (alkkagiRunner) M.Runner.stop(alkkagiRunner);
      if (alkkagiRender) {
        M.Render.stop(alkkagiRender);
        if (alkkagiRender.canvas && alkkagiRender.canvas.parentNode) {
          alkkagiRender.canvas.remove();
        }
      }
      if (alkkagiWorld) M.Composite.clear(alkkagiWorld);
      alkkagiEngine = null;
      alkkagiRender = null;
      alkkagiRunner = null;
      alkkagiWorld = null;
    }
    alkkagiBodies = {};
    alkkagiInitialized = false;
  };

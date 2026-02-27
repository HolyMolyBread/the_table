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
  let alkkagiDragging = false;
  let alkkagiDragStone = null;
  let alkkagiStartPos = { x: 0, y: 0 };
  let alkkagiCurrentPos = { x: 0, y: 0 };
  window.alkkagiJustFlicked = false;

  const ALKKAGI_W = 420, ALKKAGI_H = 420;
  const ALKKAGI_CELL = 28;

  function showAlkkagiUI() {
    switchGameView('alkkagi');
  }

  function renderAlkkagi(data) {
    if (!data) return;
    alkkagiPhase = data.phase || 'ready';
    const players = data.players || [];
    if (players[0] === currentUserId) alkkagiMyColor = 1;
    else if (players[1] === currentUserId) alkkagiMyColor = 2;
    else alkkagiMyColor = 0;
    alkkagiMyTurn = (data.currentTurn === currentUserId);

    const statusEl = document.getElementById('alkkagi-status');
    const phaseEl = document.getElementById('alkkagi-phase');
    const stonesEl = document.getElementById('alkkagi-stones-count');
    if (statusEl) {
      if (alkkagiPhase === 'placement') {
        statusEl.textContent = '배치 중 — 자기 영역의 격자를 클릭하여 돌을 놓으세요';
      } else if (alkkagiPhase === 'playing') {
        const turn = data.currentTurn || '';
        statusEl.textContent = turn === currentUserId
          ? '🎯 내 차례 — 돌을 당겨 쏘세요!'
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
      const black = stones.filter(s => s.color === 1).length;
      const white = stones.filter(s => s.color === 2).length;
      stonesEl.textContent = stones.length ? `흑 ${black} : 백 ${white}` : '흑 5 : 백 5';
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
    canvas.style.cssText = 'display:block; cursor:pointer; border-radius:8px;';
    container.appendChild(canvas);
    alkkagiPlacementCanvas = canvas;
    updateAlkkagiPlacement(data);

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
    const ctx = alkkagiPlacementCanvas.getContext('2d');
    ctx.fillStyle = '#c8a45a';
    ctx.fillRect(0, 0, ALKKAGI_W, ALKKAGI_H);
    ctx.strokeStyle = 'rgba(122,80,16,0.4)';
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
      ctx.fillStyle = s.color === 1 ? '#111' : '#f5f5f5';
      ctx.strokeStyle = s.color === 1 ? '#333' : '#ccc';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(s.x, s.y, 18, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
    });
  }

  function initAlkkagiPhysics(container, data) {
    const M = Matter;
    const W = ALKKAGI_W, H = ALKKAGI_H;

    const engine = M.Engine.create();
    engine.gravity.x = 0;
    engine.gravity.y = 0;

    const world = engine.world;

    const stoneRadius = 18;
    const stoneOpts = { friction: 0.01, frictionAir: 0.008, restitution: 0.6, density: 0.001 };

    const stones = data.stones || [];
    stones.forEach(s => {
      const fill = s.color === 1 ? '#111' : '#f5f5f5';
      const stroke = s.color === 1 ? '#333' : '#ccc';
      const body = M.Bodies.circle(s.x || 100, s.y || 100, stoneRadius, {
        ...stoneOpts,
        render: { fillStyle: fill, strokeStyle: stroke, lineWidth: 2 },
      });
      body.alkkagiId = s.id;
      body.alkkagiColor = s.color;
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
      ctx.strokeStyle = 'rgba(122,80,16,0.4)';
      ctx.lineWidth = 1;
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
      if (!alkkagiDragging || !alkkagiDragStone) return;
      const ctx = render.context;
      const body = alkkagiDragStone;
      const dx = alkkagiStartPos.x - alkkagiCurrentPos.x;
      const dy = alkkagiStartPos.y - alkkagiCurrentPos.y;
      const len = Math.sqrt(dx * dx + dy * dy) || 1;
      const ux = dx / len;
      const uy = dy / len;
      const lineLen = Math.min(len * 2, 80);
      ctx.strokeStyle = '#c00';
      ctx.lineWidth = 3;
      ctx.beginPath();
      ctx.moveTo(body.position.x, body.position.y);
      ctx.lineTo(body.position.x + ux * lineLen, body.position.y + uy * lineLen);
      ctx.stroke();
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
          const freq = Math.min(880, 440 + speed * 80);
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

    render.canvas.addEventListener('pointerdown', function(e) {
      const pos = canvasToWorld(e);
      if (!alkkagiMyTurn || alkkagiMyColor === 0 || !allStonesStopped()) return;
      const bodies = Object.values(alkkagiBodies).filter(b => b && b.alkkagiColor === alkkagiMyColor);
      const hit = M.Query.point(bodies, pos);
      if (hit.length > 0) {
        alkkagiDragging = true;
        alkkagiDragStone = hit[0];
        alkkagiStartPos = { x: pos.x, y: pos.y };
        alkkagiCurrentPos = { x: pos.x, y: pos.y };
      }
    });

    window.addEventListener('pointermove', function(e) {
      if (alkkagiDragging) {
        alkkagiCurrentPos = canvasToWorld(e);
      }
    });

    function handlePointerUp(e) {
      if (!alkkagiDragging || !alkkagiDragStone) return;
      const pos = canvasToWorld(e);
      alkkagiCurrentPos = { x: pos.x, y: pos.y };
      const dx = alkkagiStartPos.x - alkkagiCurrentPos.x;
      const dy = alkkagiStartPos.y - alkkagiCurrentPos.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const maxForce = 0.08;
      const scale = Math.min(1, dist / 150) * maxForce / (dist || 1);
      const fx = dx * scale;
      const fy = dy * scale;
      if (typeof sendGameAction === 'function') {
        sendGameAction({
          cmd: 'flick',
          id: alkkagiDragStone.alkkagiId,
          forceX: fx,
          forceY: fy,
        });
      }
      window.alkkagiJustFlicked = true;
      alkkagiDragging = false;
      alkkagiDragStone = null;
    }

    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);

    alkkagiEngine = engine;
    alkkagiRender = render;
    alkkagiRunner = runner;
    alkkagiWorld = world;
  }

  function syncAlkkagiStones(stones) {
    if (!alkkagiWorld || !alkkagiBodies) return;
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
      if (body) {
        Matter.Body.setPosition(body, { x: s.x, y: s.y });
        Matter.Body.setVelocity(body, { x: s.velX || 0, y: s.velY || 0 });
      } else {
        const M = Matter;
        const fill = s.color === 1 ? '#111' : '#f5f5f5';
        const stroke = s.color === 1 ? '#333' : '#ccc';
        body = M.Bodies.circle(s.x || 100, s.y || 100, 18, {
          friction: 0.01, frictionAir: 0.008, restitution: 0.6, density: 0.001,
          render: { fillStyle: fill, strokeStyle: stroke, lineWidth: 2 },
        });
        body.alkkagiId = s.id;
        body.alkkagiColor = s.color;
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
    if (alkkagiPlacementCanvas) {
      alkkagiPlacementCanvas.remove();
      alkkagiPlacementCanvas = null;
    }
    if (!alkkagiEngine) return;
    const M = Matter;
    if (alkkagiRunner) M.Runner.stop(alkkagiRunner);
    if (alkkagiRender) M.Render.stop(alkkagiRender);
    if (alkkagiWorld) M.Composite.clear(alkkagiWorld);
    alkkagiBodies = {};
    alkkagiInitialized = false;
    alkkagiEngine = null;
    alkkagiRender = null;
    alkkagiRunner = null;
    alkkagiWorld = null;
    alkkagiDragging = false;
    alkkagiDragStone = null;
    alkkagiPhase = 'ready';
    window.alkkagiJustFlicked = false;
  };

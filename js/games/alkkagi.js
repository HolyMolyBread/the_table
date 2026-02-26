  // ── Alkkagi UI (Matter.js 물리 보드, 슬링샷 방식) ─────────────────────────────

  let alkkagiEngine = null;
  let alkkagiRender = null;
  let alkkagiRunner = null;
  let alkkagiWorld = null;
  let alkkagiBodies = {};  // id -> Matter.Body
  let alkkagiInitialized = false;

  let alkkagiMyColor = 0;
  let alkkagiMyTurn = false;
  let alkkagiDragging = false;
  let alkkagiDragStone = null;
  let alkkagiStartPos = { x: 0, y: 0 };
  let alkkagiCurrentPos = { x: 0, y: 0 };
  window.alkkagiJustFlicked = false;

  function showAlkkagiUI() {
    switchGameView('alkkagi');
  }

  function renderAlkkagi(data) {
    if (!data) return;
    const players = data.players || [];
    if (players[0] === currentUserId) alkkagiMyColor = 1;
    else if (players[1] === currentUserId) alkkagiMyColor = 2;
    else alkkagiMyColor = 0;
    alkkagiMyTurn = (data.currentTurn === currentUserId);

    const statusEl = document.getElementById('alkkagi-status');
    if (statusEl) {
      const turn = data.currentTurn || '';
      statusEl.textContent = turn === currentUserId
        ? '🎯 내 차례 — 돌을 당겨 쏘세요!'
        : turn ? `⏳ ${escapeHTML(turn)}의 차례` : '알까기 — 상대방을 기다리는 중...';
    }

    const wrap = document.getElementById('alkkagi-board-wrap');
    if (!wrap || typeof Matter === 'undefined') return;

    if (!alkkagiInitialized) {
      alkkagiInitialized = true;
      initAlkkagiPhysics(wrap, data);
    } else {
      syncAlkkagiStones(data.stones || []);
    }
  }

  function initAlkkagiPhysics(container, data) {
    const M = Matter;
    const W = 420, H = 420;

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

    // 바둑판 격자 선 (15x15)
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

    // 조준선 (당긴 반대 방향)
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
    Matter.Body.applyForce(body, body.position, { x: data.forceX || 0, y: data.forceY || 0 });
  };

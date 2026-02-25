  // ── Alkkagi UI (Matter.js 물리 보드) ────────────────────────────────────────

  let alkkagiEngine = null;
  let alkkagiRender = null;
  let alkkagiRunner = null;
  let alkkagiWorld = null;
  let alkkagiBodies = {};  // id -> Matter.Body
  let alkkagiInitialized = false;

  function showAlkkagiUI() {
    switchGameView('alkkagi');
  }

  function renderAlkkagi(data) {
    if (!data) return;
    const statusEl = document.getElementById('alkkagi-status');
    if (statusEl) {
      const turn = data.currentTurn || '';
      statusEl.textContent = turn === currentUserId
        ? '🎯 내 차례 — 돌을 튕겨 상대 알을 밀어내세요!'
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
    const wallThick = 15;

    const engine = M.Engine.create();
    engine.gravity.x = 0;
    engine.gravity.y = 0;

    const world = engine.world;

    // 4면 벽 (정적 바디)
    const walls = [
      M.Bodies.rectangle(W/2, -wallThick/2, W + wallThick*2, wallThick, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(W/2, H + wallThick/2, W + wallThick*2, wallThick, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(-wallThick/2, H/2, wallThick, H + wallThick*2, { isStatic: true, render: { fillStyle: '#7a5010' } }),
      M.Bodies.rectangle(W + wallThick/2, H/2, wallThick, H + wallThick*2, { isStatic: true, render: { fillStyle: '#7a5010' } }),
    ];
    M.World.add(world, walls);

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
        width: W + wallThick*2,
        height: H + wallThick*2,
        wireframes: false,
        background: '#c8a45a',
      },
    });
    M.Render.run(render);

    const mouseConstraint = M.MouseConstraint.create(engine, {
      element: render.canvas,
      constraint: { stiffness: 0.2, render: { visible: false } },
    });
    M.World.add(world, mouseConstraint);

    const runner = M.Runner.create();
    M.Runner.run(runner, engine);

    alkkagiEngine = engine;
    alkkagiRender = render;
    alkkagiRunner = runner;
    alkkagiWorld = world;
  }

  function syncAlkkagiStones(stones) {
    if (!alkkagiWorld || !alkkagiBodies) return;
    stones.forEach(s => {
      const body = alkkagiBodies[s.id];
      if (body) {
        Matter.Body.setPosition(body, { x: s.x, y: s.y });
        Matter.Body.setVelocity(body, { x: s.velX || 0, y: s.velY || 0 });
      }
    });
  }
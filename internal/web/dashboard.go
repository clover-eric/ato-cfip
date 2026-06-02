package web

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f7fb;
      --panel: #ffffff;
      --ink: #172033;
      --muted: #667085;
      --line: #d9e2f2;
      --blue: #2563eb;
      --cyan: #0891b2;
      --green: #059669;
      --red: #dc2626;
      --shadow: 0 18px 45px rgba(31, 41, 55, .10);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background:
        radial-gradient(circle at 15% 10%, rgba(37, 99, 235, .12), transparent 28%),
        radial-gradient(circle at 85% 5%, rgba(8, 145, 178, .12), transparent 26%),
        var(--bg);
      color: var(--ink);
      font: 15px/1.55 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    .wrap { width: min(1180px, calc(100% - 32px)); margin: 0 auto; padding: 28px 0 44px; }
    header { display: flex; justify-content: space-between; gap: 20px; align-items: center; margin-bottom: 22px; }
    .brand { display: flex; align-items: center; gap: 14px; }
    .mark {
      width: 46px; height: 46px; border-radius: 8px;
      background: linear-gradient(135deg, var(--blue), var(--cyan));
      display: grid; place-items: center;
      color: white; font-weight: 800; letter-spacing: 0;
      box-shadow: var(--shadow);
    }
    h1 { margin: 0; font-size: 24px; letter-spacing: 0; }
    .sub { color: var(--muted); margin-top: 2px; }
    .actions { display: flex; gap: 10px; flex-wrap: wrap; justify-content: flex-end; }
    button, .linkbtn {
      border: 1px solid var(--line);
      background: var(--panel);
      color: var(--ink);
      border-radius: 8px;
      padding: 10px 14px;
      cursor: pointer;
      text-decoration: none;
      font-weight: 650;
      min-height: 40px;
    }
    button.primary { background: var(--blue); border-color: var(--blue); color: white; }
    button.is-busy { opacity: .75; }
    .hero {
      background: linear-gradient(135deg, #ffffff 0%, #f2f7ff 55%, #e9fbff 100%);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      padding: 26px;
      display: grid;
      grid-template-columns: 1.3fr .7fr;
      gap: 22px;
      margin-bottom: 18px;
    }
    .domain {
      font-size: clamp(28px, 5vw, 52px);
      line-height: 1.05;
      font-weight: 850;
      letter-spacing: 0;
      overflow-wrap: anywhere;
      margin: 8px 0 12px;
    }
    .badge {
      display: inline-flex; align-items: center; gap: 8px;
      border: 1px solid var(--line);
      background: rgba(255,255,255,.75);
      padding: 6px 10px;
      border-radius: 999px;
      color: var(--muted);
      font-weight: 650;
    }
    .dot { width: 8px; height: 8px; border-radius: 99px; background: var(--green); }
    .dot.running { background: var(--cyan); }
    .dot.error { background: var(--red); }
    .stats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
    .stat {
      background: rgba(255,255,255,.72);
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 14px;
    }
    .stat .k { color: var(--muted); font-size: 13px; }
    .stat .v { font-size: 23px; font-weight: 800; margin-top: 3px; overflow-wrap: anywhere; }
    .progress {
      margin-top: 14px;
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 10px;
    }
    .mini { border-top: 1px dashed var(--line); padding-top: 10px; }
    .mini .k { color: var(--muted); font-size: 12px; }
    .mini .v { font-weight: 750; overflow-wrap: anywhere; }
    .grid { display: grid; grid-template-columns: 1fr 360px; gap: 18px; align-items: start; }
    section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      overflow: hidden;
    }
    .section-head {
      padding: 16px 18px;
      border-bottom: 1px solid var(--line);
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    h2 { margin: 0; font-size: 17px; letter-spacing: 0; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 12px 14px; border-bottom: 1px solid var(--line); white-space: nowrap; }
    th { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; background: #fbfcff; }
    td:first-child, th:first-child { padding-left: 18px; }
    tr:last-child td { border-bottom: 0; }
    .ip { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-weight: 750; }
    .speed { color: var(--green); font-weight: 800; }
    .side { display: grid; gap: 18px; }
    .kv { padding: 16px 18px; display: grid; gap: 12px; }
    .row { display: flex; justify-content: space-between; gap: 14px; border-bottom: 1px dashed var(--line); padding-bottom: 10px; }
    .row:last-child { border-bottom: 0; padding-bottom: 0; }
    .row span:first-child { color: var(--muted); }
    .row span:last-child { font-weight: 750; text-align: right; overflow-wrap: anywhere; }
    .empty { padding: 28px 18px; color: var(--muted); }
    .notice { margin-top: 14px; color: var(--muted); max-width: 760px; }
    .domain-tools { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; margin-top: 12px; }
    .domain-tools input {
      min-width: min(420px, 100%);
      flex: 1;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px 12px;
      font: inherit;
      color: var(--ink);
      background: rgba(255,255,255,.86);
    }
    .domain-tools[hidden] { display: none; }
    .foot { color: var(--muted); margin-top: 18px; font-size: 13px; }
    @media (max-width: 860px) {
      header, .hero { grid-template-columns: 1fr; display: grid; }
      .grid { grid-template-columns: 1fr; }
      .actions { justify-content: start; }
      .stats, .progress { grid-template-columns: 1fr; }
      table { display: block; overflow-x: auto; }
      .wrap { width: min(100% - 22px, 1180px); padding-top: 18px; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <header>
      <div class="brand">
        <div class="mark">CF</div>
        <div>
          <h1>{{.Title}}</h1>
          <div class="sub">24 小时自动测速与优选域名发布</div>
        </div>
      </div>
      <div class="actions">
        <a class="linkbtn" href="/json" target="_blank">JSON</a>
        <button class="primary" id="runBtn">立即测速</button>
      </div>
    </header>

    <div class="hero">
      <div>
        <div class="badge"><span class="dot" id="stateDot"></span><span id="stateText">加载中</span></div>
        <div class="domain" id="domainText">{{.Domain}}</div>
        <div class="sub">将客户端 DNS 或业务域名指向这里发布的优选 IP。注意：DNS 域名不能包含端口。</div>
        <div class="domain-tools">
          <button id="editDomainBtn" type="button">修改域名</button>
        </div>
        <div class="domain-tools" id="domainEditor" hidden>
          <input id="domainInput" type="text" spellcheck="false" placeholder="ip.is33.cn 或 https://ip.is33.cn:666/">
          <button id="saveDomainBtn" type="button">保存</button>
          <button id="cancelDomainBtn" type="button">取消</button>
        </div>
        <div class="notice" id="stageText">等待状态。</div>
        <div class="progress">
          <div class="mini"><div class="k">轮次</div><div class="v" id="roundNow">-</div></div>
          <div class="mini"><div class="k">已选</div><div class="v" id="selectedNow">-</div></div>
          <div class="mini"><div class="k">候选</div><div class="v" id="candidateNow">-</div></div>
          <div class="mini"><div class="k">耗时</div><div class="v" id="elapsedNow">-</div></div>
        </div>
      </div>
      <div class="stats">
        <div class="stat"><div class="k">已发布 IP</div><div class="v" id="count">0</div></div>
        <div class="stat"><div class="k">目标数量</div><div class="v" id="target">-</div></div>
        <div class="stat"><div class="k">最快速度</div><div class="v" id="topSpeed">-</div></div>
        <div class="stat"><div class="k">最近运行</div><div class="v" id="lastRun">-</div></div>
      </div>
    </div>

    <div class="grid">
      <section>
        <div class="section-head">
          <h2>当前优选 IP</h2>
          <span class="sub" id="generatedAt">等待首次发布</span>
        </div>
        <div id="tableWrap" class="empty">还没有发布结果。可以点击“立即测速”，或等待定时任务自动运行。</div>
      </section>

      <div class="side">
        <section>
          <div class="section-head"><h2>运行配置</h2></div>
          <div class="kv" id="configBox"></div>
        </section>
        <section>
          <div class="section-head"><h2>部署提示</h2></div>
          <div class="kv">
            <div class="row"><span>DNS 记录</span><span>仅 DNS / 灰云</span></div>
            <div class="row"><span>面板地址</span><span>域名:端口</span></div>
            <div class="row"><span>配置变量</span><span>ATO_DOMAIN</span></div>
          </div>
        </section>
      </div>
    </div>
    <div class="foot">页面每 5 秒自动刷新。完整测速会执行多轮任务，可能需要几分钟。</div>
  </div>

  <script>
    const fmt = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' });
    const runBtn = document.getElementById('runBtn');
    const domainText = document.getElementById('domainText');
    const editDomainBtn = document.getElementById('editDomainBtn');
    const domainEditor = document.getElementById('domainEditor');
    const domainInput = document.getElementById('domainInput');
    const saveDomainBtn = document.getElementById('saveDomainBtn');
    const cancelDomainBtn = document.getElementById('cancelDomainBtn');
    let currentDomain = '{{.Domain}}';

    editDomainBtn.addEventListener('click', () => {
      domainInput.value = currentDomain;
      domainEditor.hidden = false;
      editDomainBtn.hidden = true;
      domainInput.focus();
      domainInput.select();
    });
    cancelDomainBtn.addEventListener('click', () => {
      domainEditor.hidden = true;
      editDomainBtn.hidden = false;
    });
    saveDomainBtn.addEventListener('click', saveDomain);
    domainInput.addEventListener('keydown', event => {
      if (event.key === 'Enter') saveDomain();
      if (event.key === 'Escape') cancelDomainBtn.click();
    });

    async function saveDomain() {
      saveDomainBtn.classList.add('is-busy');
      saveDomainBtn.textContent = '保存中';
      try {
        const res = await fetch('/api/config/domain', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ domain: domainInput.value })
        });
        if (!res.ok) {
          throw new Error(await res.text());
        }
        const data = await res.json();
        currentDomain = data.domain;
        domainText.textContent = data.domain;
        document.getElementById('stageText').textContent = data.note || '域名已保存。';
        domainEditor.hidden = true;
        editDomainBtn.hidden = false;
        refresh();
      } catch (err) {
        document.getElementById('stageText').textContent = '保存失败：' + err.message;
      } finally {
        saveDomainBtn.classList.remove('is-busy');
        saveDomainBtn.textContent = '保存';
      }
    }

    runBtn.addEventListener('click', async () => {
      if (runBtn.dataset.pending === 'true') return;
      runBtn.dataset.pending = 'true';
      runBtn.classList.add('is-busy');
      runBtn.textContent = '启动中';
      try {
        await fetch('/api/run', { method: 'POST' });
      } finally {
        setTimeout(() => {
          runBtn.dataset.pending = 'false';
          refresh();
        }, 800);
      }
    });

    async function refresh() {
      const res = await fetch('/api/status', { cache: 'no-store' });
      const data = await res.json();
      const st = data.status;
      const cfg = data.config;
      const ips = st.published && st.published.ips ? st.published.ips : [];
      currentDomain = cfg.domain || currentDomain;
      domainText.textContent = currentDomain;

      document.getElementById('count').textContent = ips.length;
      document.getElementById('target').textContent = st.target || cfg.desired_unique_ips || '-';
      document.getElementById('topSpeed').textContent = ips.length ? Number(ips[0].download_mbps).toFixed(2) + ' MB/s' : '-';
      document.getElementById('lastRun').textContent = st.last_ended ? fmt.format(new Date(st.last_ended)) : '-';
      document.getElementById('generatedAt').textContent = st.published && st.published.generated_at ? '发布于 ' + fmt.format(new Date(st.published.generated_at)) : '等待首次发布';
      document.getElementById('roundNow').textContent = st.current_round ? st.current_round + ' / ' + st.rounds : '-';
      document.getElementById('selectedNow').textContent = (st.selected || 0) + ' / ' + (st.target || cfg.desired_unique_ips || '-');
      document.getElementById('candidateNow').textContent = st.candidates || 0;
      document.getElementById('elapsedNow').textContent = st.elapsed_sec ? st.elapsed_sec + ' 秒' : '-';
      let stageText = translateStage(st.stage || (st.running ? 'Testing in progress.' : 'Idle.'));
      if (st.zero_speed) {
        stageText += '；零速候选 ' + st.zero_speed + ' 个，请确认测速地址可访问';
      }
      document.getElementById('stageText').textContent = stageText;

      const dot = document.getElementById('stateDot');
      const txt = document.getElementById('stateText');
      dot.className = 'dot';
      if (st.running) {
        dot.classList.add('running');
        txt.textContent = '正在测速';
        runBtn.classList.add('is-busy');
        runBtn.textContent = '测速中';
      } else if (st.last_error) {
        dot.classList.add('error');
        txt.textContent = '异常';
        runBtn.classList.remove('is-busy');
        runBtn.textContent = '重新测速';
      } else {
        txt.textContent = ips.length ? '运行正常' : '暂无结果';
        runBtn.classList.remove('is-busy');
        runBtn.textContent = '立即测速';
      }
      renderTable(ips);
      renderConfig(cfg);
    }

    function renderTable(ips) {
      const wrap = document.getElementById('tableWrap');
      if (!ips.length) {
        wrap.className = 'empty';
        wrap.textContent = '还没有发布结果。如果完整运行后仍无结果，请打开 /json 查看 last_error，并确认测速地址可访问。';
        return;
      }
      wrap.className = '';
      wrap.innerHTML = '<table><thead><tr><th>#</th><th>IP</th><th>速度</th><th>延迟</th><th>丢包</th><th>节点</th><th>轮次</th></tr></thead><tbody>' +
        ips.map((ip, i) => '<tr><td>' + (i + 1) + '</td><td class="ip">' + escapeHTML(ip.ip) + '</td><td class="speed">' + Number(ip.download_mbps).toFixed(2) + ' MB/s</td><td>' + Number(ip.delay_ms).toFixed(2) + ' ms</td><td>' + (Number(ip.loss_rate) * 100).toFixed(0) + '%</td><td>' + escapeHTML(ip.colo || 'N/A') + '</td><td>' + ip.round + '</td></tr>').join('') +
        '</tbody></table>';
    }

    function renderConfig(cfg) {
      document.getElementById('configBox').innerHTML = [
        ['发布模式', cfg.publish_mode],
        ['域名', cfg.domain],
        ['计划任务', cfg.schedule],
        ['测速轮次', cfg.rounds_per_hour],
        ['下载时长', cfg.download_time + ' 秒'],
        ['测速地址', cfg.download_url],
        ['监听地址', cfg.web_listen]
      ].map(([k, v]) => '<div class="row"><span>' + k + '</span><span>' + escapeHTML(String(v)) + '</span></div>').join('');
    }

    function translateStage(stage) {
      if (!stage) return '';
      return stage
        .replace('Preparing', '准备测速')
        .replace('Scanning candidate IPs', '正在扫描候选 IP')
        .replace('Testing in progress.', '正在测速')
        .replace('Idle.', '空闲')
        .replace('Finished', '运行完成')
        .replace('no publishable IPs found', '没有找到可发布的 IP')
        .replace(/Round (\d+) produced no usable result/g, '第 $1 轮没有可用结果')
        .replace(/Round (\d+) failed:/g, '第 $1 轮失败：')
        .replace(/Round (\d+) was duplicated, continuing/g, '第 $1 轮结果重复，继续补测')
        .replace(/Selected (\d+)\/(\d+) IPs/g, '已选出 $1/$2 个 IP')
        .replace(/; (\d+) candidates had 0 MB\/s download/g, '；$1 个候选下载速度为 0');
    }

    function escapeHTML(v) {
      return v.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    }

    refresh();
    setInterval(refresh, 5000);
  </script>
</body>
</html>`

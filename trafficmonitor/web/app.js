const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];

const state = {
  view: 'overview',
  trendMode: 'route',
  summary: emptySummary(),
  rows: [],
  points: [],
  proxyDomains: [],
  nodeRegions: [],
  candidates: [],
  requestID: 0,
  controller: null,
  sort: 'total',
  order: 'desc',
  searchContext: 'overview',
  searches: { overview: '', domain: '', ip: '', country: '', node: '', node_region: '', proxy: '', rule: '', process: '', candidates: '' }
};

const dimensionLabels = {
  domain: '域名',
  ip: '目标 IP',
  country: '目标 GeoIP',
  node: '节点',
  node_region: '节点地区',
  proxy: '代理链',
  rule: '规则类型',
  process: '进程'
};

const dimensionTabLabels = {
  domain: '域名流量',
  ip: 'IP 流量',
  country: '目标 GeoIP',
  node: '节点流量',
  node_region: '节点地区流量',
  proxy: '代理链流量',
  rule: '规则流量',
  process: '进程流量'
};

const dimensionNotes = {
  country: ' · 仅记录 Mihomo 已查询到的 GeoIP 标签；升级前数据及未查询连接显示未知',
  node_region: ' · 根据节点名称归类；无法识别归其他，DIRECT / REJECT 单列'
};

const sortLabels = {
  total: '总流量',
  upload: '上传流量',
  download: '下载流量',
  proxy: '代理流量',
  direct: '直连流量',
  reject: '拒绝流量',
  connections: '活跃计数'
};

const routeSorts = ['proxy', 'direct', 'reject'];

function emptySummary() {
  return { uploadBytes: 0, downloadBytes: 0, proxyBytes: 0, directBytes: 0, rejectBytes: 0 };
}

function formatBytes(value) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let amount = bytes / 1024;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) { amount /= 1024; index += 1; }
  return `${amount.toFixed(amount >= 100 ? 0 : 1)} ${units[index]}`;
}

function formatCount(value) {
  return Math.max(0, Number(value) || 0).toLocaleString();
}

function formatDateTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '时间未知';
  return date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function escapeHTML(value) {
  return String(value == null ? '' : value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  })[character]);
}

async function api(path, signal) {
  const response = await fetch(path, { signal });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `请求失败 (${response.status})`);
  return body;
}

function overviewParams() {
  return new URLSearchParams({
    dimension: 'domain',
    minutes: $('#minutes').value,
    route: $('#route').value,
    search: ''
  });
}

function rankingParams(limit = 100) {
  return new URLSearchParams({
    dimension: $('#dimension').value,
    minutes: $('#minutes').value,
    route: $('#route').value,
    search: $('#search').value.trim(),
    sort: state.sort,
    order: state.order,
    limit: String(limit)
  });
}

function proxyOverviewParams(dimension, limit) {
  return new URLSearchParams({
    dimension,
    minutes: $('#minutes').value,
    route: 'proxy',
    search: '',
    sort: 'proxy',
    order: 'desc',
    limit: String(limit)
  });
}

function percentageValue(value, total) {
  return total > 0 ? Math.max(0, Number(value) || 0) / total * 100 : 0;
}

function percentage(value, total) {
  return `${percentageValue(value, total).toFixed(1)}%`;
}

function bucketLabel() {
  const minutes = Number($('#minutes').value);
  if (minutes <= 60) return '每 5 分钟';
  if (minutes <= 360) return '每 15 分钟';
  if (minutes <= 1440) return '每小时';
  if (minutes <= 10080) return '每 6 小时';
  return '每 24 小时';
}

async function loadOverview(signal, requestID) {
  const params = overviewParams();
  const domainParams = proxyOverviewParams('domain', 5);
  const regionParams = proxyOverviewParams('node_region', 100);
  const [summary, points, proxyDomains, nodeRegions] = await Promise.all([
    api(`/api/summary?${params}`, signal),
    api(`/api/timeseries?${params}`, signal),
    api(`/api/traffic?${domainParams}`, signal),
    api(`/api/traffic?${regionParams}`, signal)
  ]);
  if (requestID !== state.requestID) return;
  state.summary = summary;
  state.points = points;
  state.proxyDomains = proxyDomains;
  state.nodeRegions = nodeRegions;
  renderSummary();
  renderTrend();
  renderRouteChart();
  renderOverviewRankings();
}

async function loadRanking(signal, requestID) {
  const params = rankingParams(100);
  const [summary, rows] = await Promise.all([
    api(`/api/summary?${params}`, signal),
    api(`/api/traffic?${params}`, signal)
  ]);
  if (requestID !== state.requestID) return;
  state.summary = summary;
  state.rows = rows;
  renderRanking();
}

function renderSummary() {
  const summary = state.summary;
  const total = summary.uploadBytes + summary.downloadBytes;
  const routedTotal = summary.proxyBytes + summary.directBytes + summary.rejectBytes;
  $('#total-bytes').textContent = formatBytes(total);
  $('#total-detail').textContent = `上传 ${formatBytes(summary.uploadBytes)} · 下载 ${formatBytes(summary.downloadBytes)}`;
  $('#proxy-bytes').textContent = formatBytes(summary.proxyBytes);
  $('#direct-bytes').textContent = formatBytes(summary.directBytes);
  $('#reject-bytes').textContent = formatBytes(summary.rejectBytes);
  $('#proxy-ratio').textContent = `占总流量 ${percentage(summary.proxyBytes, routedTotal)}`;
  $('#direct-ratio').textContent = `占总流量 ${percentage(summary.directBytes, routedTotal)}`;
  $('#reject-ratio').textContent = `占总流量 ${percentage(summary.rejectBytes, routedTotal)}`;
}

function linePath(values, x, y) {
  return values.map((value, index) => `${index === 0 ? 'M' : 'L'} ${x(index).toFixed(2)} ${y(value).toFixed(2)}`).join(' ');
}

function trendSeries() {
  if (state.trendMode === 'direction') {
    return [
      { key: 'download', label: '下载', values: state.points.map((point) => point.downloadBytes) },
      { key: 'upload', label: '上传', values: state.points.map((point) => point.uploadBytes) }
    ];
  }
  return [
    { key: 'proxy', label: '代理', values: state.points.map((point) => point.proxyBytes) },
    { key: 'direct', label: '直连', values: state.points.map((point) => point.directBytes) },
    { key: 'reject', label: '拒绝', values: state.points.map((point) => point.rejectBytes) }
  ];
}

function renderTrend() {
  const chart = $('#trend-chart');
  const series = trendSeries();
  $('#trend-legend').innerHTML = series.map((item) => `<span><i class="${item.key}"></i>${item.label}</span>`).join('');
  $('#trend-description').textContent = `${bucketLabel()}聚合 · 数据截至上一完整分钟`;

  const activity = series.reduce((sum, item) => sum + item.values.reduce((itemSum, value) => itemSum + value, 0), 0);
  if (!state.points.length || activity === 0) {
    chart.innerHTML = '<div class="empty">当前筛选条件下暂无趋势数据</div>';
    return;
  }

  const width = Math.max(320, Math.round(chart.clientWidth - 20));
  const height = Math.max(170, Math.round(chart.clientHeight - 15));
  const left = 52, right = 10, top = 11, bottom = 27;
  const chartWidth = width - left - right;
  const chartHeight = height - top - bottom;
  const max = Math.max(1, ...series.flatMap((item) => item.values));
  const x = (index) => left + (state.points.length === 1 ? chartWidth / 2 : index / (state.points.length - 1) * chartWidth);
  const y = (value) => top + chartHeight - value / max * chartHeight;

  let grid = '';
  for (let index = 0; index <= 4; index += 1) {
    const value = max * (4 - index) / 4;
    const lineY = top + chartHeight * index / 4;
    grid += `<line class="grid-line" x1="${left}" y1="${lineY}" x2="${width - right}" y2="${lineY}"></line><text class="axis-label" x="${left - 6}" y="${lineY + 3}" text-anchor="end">${formatBytes(value)}</text>`;
  }

  const labelFractions = width >= 560 ? [0, 1 / 3, 2 / 3, 1] : [0, 1 / 2, 1];
  const labelIndexes = [...new Set(labelFractions.map((fraction) => Math.round((state.points.length - 1) * fraction)))];
  const timeLabels = labelIndexes.map((index) => {
    const date = new Date(state.points[index].timestamp * 1000);
    const label = Number($('#minutes').value) > 1440
      ? date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', hour12: false })
      : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return `<text class="axis-label" x="${x(index)}" y="${height - 7}" text-anchor="middle">${label}</text>`;
  }).join('');

  const firstPath = linePath(series[0].values, x, y);
  const areaPath = `${firstPath} L ${x(state.points.length - 1)} ${top + chartHeight} L ${x(0)} ${top + chartHeight} Z`;
  const paths = series.map((item) => `<path class="series-line ${item.key}" d="${linePath(item.values, x, y)}"></path>`).join('');
  const hitWidth = Math.max(8, chartWidth / Math.max(1, state.points.length));
  const hitAreas = state.points.map((point, index) => {
    const title = `${new Date(point.timestamp * 1000).toLocaleString()}\n${series.map((item) => `${item.label} ${formatBytes(item.values[index])}`).join(' · ')}`;
    return `<rect class="chart-hit" x="${Math.max(left, x(index) - hitWidth / 2)}" y="${top}" width="${hitWidth}" height="${chartHeight}"><title>${escapeHTML(title)}</title></rect>`;
  }).join('');

  chart.innerHTML = `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="流量趋势图">${grid}<path class="series-area ${series[0].key}" d="${areaPath}"></path>${paths}${timeLabels}${hitAreas}</svg>`;
}

function renderRouteChart() {
  const summary = state.summary;
  const routes = [
    { key: 'proxy', label: '代理', value: summary.proxyBytes },
    { key: 'direct', label: '直连', value: summary.directBytes },
    { key: 'reject', label: '拒绝', value: summary.rejectBytes }
  ];
  const total = routes.reduce((sum, route) => sum + route.value, 0);
  const selectedRoute = $('#route').value;
  const highlightedRoute = routes.find((route) => route.key === selectedRoute);
  const highlightLabel = highlightedRoute ? `${highlightedRoute.label}流量` : '代理占比';
  const highlightValue = highlightedRoute ? formatBytes(highlightedRoute.value) : percentage(summary.proxyBytes, total);
  const highlightDetail = highlightedRoute ? `当前仅统计${highlightedRoute.label}路径` : `路径总量 ${formatBytes(total)}`;
  const bars = routes.map((route) => `<i class="${route.key}" style="width:${percentageValue(route.value, total)}%"></i>`).join('');
  const items = routes.map((route) => `<div class="route-item"><i class="route-dot ${route.key}"></i><span>${route.label}</span><small>${percentage(route.value, total)}</small><strong>${formatBytes(route.value)}</strong></div>`).join('');
  $('#route-chart').innerHTML = `<div class="route-highlight"><div><span>${highlightLabel}</span><strong>${highlightValue}</strong></div><small>${highlightDetail}</small></div><div class="stacked-bar">${bars}</div><div class="route-list">${items}</div>`;
}

function collapseRegionRows(rows, limit = 4) {
  const visible = rows.slice(0, limit).map((row) => ({ ...row }));
  const remainingBytes = rows.slice(limit).reduce((sum, row) => sum + (Number(row.proxyBytes) || 0), 0);
  if (remainingBytes > 0) visible.push({ key: '其余地区', proxyBytes: remainingBytes });
  return visible;
}

function renderCompactRanking(target, rows, total, accent, emptyMessage) {
  const container = $(target);
  if (!rows.length || total <= 0) {
    container.innerHTML = `<div class="empty compact-empty">${escapeHTML(emptyMessage)}</div>`;
    return;
  }
  container.innerHTML = rows.map((row, index) => {
    const bytes = Math.max(0, Number(row.proxyBytes) || 0);
    return `<div class="compact-rank-row">
      <span class="compact-rank-index">${index + 1}</span>
      <div class="compact-rank-object"><strong title="${escapeHTML(row.key)}">${escapeHTML(row.key)}</strong><div class="compact-rank-track"><i class="${accent}" style="width:${percentageValue(bytes, total)}%"></i></div></div>
      <strong class="compact-rank-value">${formatBytes(bytes)}</strong>
      <small>${percentage(bytes, total)}</small>
    </div>`;
  }).join('');
}

function renderOverviewRankings() {
  const proxyTotal = state.nodeRegions.reduce((sum, row) => sum + (Number(row.proxyBytes) || 0), 0);
  const regionRows = collapseRegionRows(state.nodeRegions);
  $('#proxy-domain-count').textContent = state.proxyDomains.length ? `Top ${state.proxyDomains.length}` : '0 项';
  $('#node-region-count').textContent = state.nodeRegions.length ? `${state.nodeRegions.length} 个分组` : '0 项';
  renderCompactRanking('#proxy-domain-report', state.proxyDomains, proxyTotal, 'domain', '当前时间范围暂无代理域名流量');
  renderCompactRanking('#node-region-report', regionRows, proxyTotal, 'region', '当前时间范围暂无代理节点地区数据');
}

function renderRanking() {
  const reportTotal = state.summary.uploadBytes + state.summary.downloadBytes;
  updateRankingLabels();
  renderSortControls();
  $('#result-count').textContent = `显示 ${state.rows.length} 项`;
  $('#ranking-body').innerHTML = state.rows.length ? state.rows.map((row, index) => {
    const routeTotal = row.proxyBytes + row.directBytes + row.rejectBytes;
    const proxyWidth = percentageValue(row.proxyBytes, routeTotal);
    const directWidth = percentageValue(row.directBytes, routeTotal);
    const rejectWidth = percentageValue(row.rejectBytes, routeTotal);
    return `<tr>
      <td class="rank">${index + 1}</td>
      <td class="object-name" title="${escapeHTML(row.key)}">${escapeHTML(row.key)}</td>
      <td class="total-cell"><strong>${formatBytes(row.totalBytes)}</strong><div class="share-line"><div class="share-track"><i style="width:${percentageValue(row.totalBytes, reportTotal)}%"></i></div><small>${percentage(row.totalBytes, reportTotal)}</small></div></td>
      <td><div class="metric-pair"><span><i>↑</i>${formatBytes(row.uploadBytes)}</span><span><i>↓</i>${formatBytes(row.downloadBytes)}</span></div></td>
      <td title="代理 ${formatBytes(row.proxyBytes)}；直连 ${formatBytes(row.directBytes)}；拒绝 ${formatBytes(row.rejectBytes)}"><div class="row-route-bar"><i class="proxy" style="width:${proxyWidth}%"></i><i class="direct" style="width:${directWidth}%"></i><i class="reject" style="width:${rejectWidth}%"></i></div><div class="route-values"><span class="route-value proxy">代理 ${formatBytes(row.proxyBytes)}</span><span class="route-value direct">直连 ${formatBytes(row.directBytes)}</span><span class="route-value reject">拒绝 ${formatBytes(row.rejectBytes)}</span></div></td>
      <td>${formatCount(row.connections)}</td>
    </tr>`;
  }).join('') : '<tr><td colspan="6" class="empty">当前筛选条件下暂无历史数据</td></tr>';
}

function updateRankingLabels() {
  const dimensionLabel = dimensionLabels[$('#dimension').value] || '对象';
  $('#ranking-title').textContent = `${dimensionLabel}流量排行`;
  $('#dimension-heading').textContent = dimensionLabel;
}

function showRankingLoading() {
  updateRankingLabels();
  renderSortControls();
  $('#result-count').textContent = '加载中';
  $('#ranking-body').innerHTML = '<tr><td colspan="6" class="empty">正在加载排行数据</td></tr>';
}

function renderSortControls() {
  const directionLabel = state.order === 'asc' ? '升序' : '降序';
  const selectedRoute = $('#route').value;
  const dimensionNote = dimensionNotes[$('#dimension').value] || '';
  $('#ranking-description').textContent = `按${sortLabels[state.sort]}${directionLabel}，最多显示 100 项${dimensionNote}`;
  const headers = new Set($$('.sort-button').map((button) => button.closest('th')));
  headers.forEach((header) => {
    header.removeAttribute('aria-sort');
    header.removeAttribute('aria-label');
  });
  $$('.sort-button').forEach((button) => {
    const active = button.dataset.sort === state.sort;
    const disabled = selectedRoute && routeSorts.includes(button.dataset.sort) && button.dataset.sort !== selectedRoute;
    button.disabled = Boolean(disabled);
    button.classList.toggle('active', active);
    button.classList.toggle('ascending', active && state.order === 'asc');
    button.classList.toggle('descending', active && state.order === 'desc');
    button.setAttribute('aria-pressed', String(active));
    const nextDirection = active && state.order === 'desc' ? '升序' : '降序';
    const ariaLabel = disabled
      ? `当前路径筛选下无法按${sortLabels[button.dataset.sort]}排序`
      : active
        ? `当前按${sortLabels[button.dataset.sort]}${directionLabel}，点击切换${nextDirection}`
        : `点击按${sortLabels[button.dataset.sort]}降序`;
    button.setAttribute('aria-label', ariaLabel);
    if (active) {
      const header = button.closest('th');
      header.setAttribute('aria-sort', state.order === 'asc' ? 'ascending' : 'descending');
      header.setAttribute('aria-label', `当前按${sortLabels[button.dataset.sort]}${directionLabel}`);
    }
  });
}

function syncSortingForRoute() {
  const selectedRoute = $('#route').value;
  if (selectedRoute && routeSorts.includes(state.sort) && state.sort !== selectedRoute) {
    state.sort = selectedRoute;
    state.order = 'desc';
  }
}

function changeSorting(sort) {
  cancelScheduledSearch();
  if (state.sort === sort) state.order = state.order === 'desc' ? 'asc' : 'desc';
  else {
    state.sort = sort;
    state.order = 'desc';
  }
  renderSortControls();
  refreshReport();
}

async function loadCandidates(signal, requestID) {
  const params = new URLSearchParams({ minutes: $('#minutes').value, search: $('#search').value.trim(), limit: '200' });
  const candidates = await api(`/api/direct-candidates?${params}`, signal);
  if (requestID !== state.requestID) return;
  state.candidates = candidates;
  renderCandidates();
}

function renderCandidates() {
  const labels = { high: '优先验证', medium: '建议验证', review: '人工判断' };
  $('#candidate-count').textContent = `Top 200 · ${state.candidates.length} 项`;
  $('#candidate-body').innerHTML = state.candidates.length ? state.candidates.map((candidate) => {
    const geo = `${candidate.countries || '未知'} / ${candidate.asns || '未知'}`;
    const history = `${candidate.nodes || '未知节点'} / ${candidate.rules || '未知规则'}`;
    return `<article class="candidate-card">
      <div class="candidate-top">
        <div class="candidate-identity"><span class="candidate-domain" title="${escapeHTML(candidate.domain)}">${escapeHTML(candidate.domain)}</span><small class="candidate-meta">最后记录 ${escapeHTML(formatDateTime(candidate.lastSeen))} · 活跃计数 ${formatCount(candidate.connections)}</small></div>
        <div class="candidate-score"><span class="confidence ${escapeHTML(candidate.confidence)}">${escapeHTML(labels[candidate.confidence] || candidate.confidence)}</span><strong>${formatBytes(candidate.totalBytes)}</strong></div>
      </div>
      <p class="candidate-reason">${escapeHTML(candidate.reason)}</p>
      <div class="candidate-facts">
        <div class="candidate-fact"><span>上传 / 下载</span><strong>${formatBytes(candidate.uploadBytes)} / ${formatBytes(candidate.downloadBytes)}</strong></div>
        <div class="candidate-fact"><span>GeoIP / ASN</span><strong title="${escapeHTML(geo)}">${escapeHTML(geo)}</strong></div>
        <div class="candidate-fact wide"><span>历史节点 / 规则类型</span><strong title="${escapeHTML(history)}">${escapeHTML(history)}</strong></div>
      </div>
      <div class="rule-row"><code title="${escapeHTML(candidate.suggestedRule)}">${escapeHTML(candidate.suggestedRule)}</code><button type="button" class="copy-button" data-rule="${escapeHTML(candidate.suggestedRule)}">复制规则</button></div>
    </article>`;
  }).join('') : '<div class="empty">当前范围没有走代理的域名记录</div>';
}

function showStatus(message) {
  const status = $('#report-status');
  status.textContent = message;
  status.classList.toggle('hidden', !message);
}

function clearCurrentView() {
  if (state.view === 'overview') {
    state.summary = emptySummary();
    state.points = [];
    state.proxyDomains = [];
    state.nodeRegions = [];
    renderSummary();
    renderTrend();
    renderRouteChart();
    renderOverviewRankings();
    return;
  }
  if (state.view === 'ranking') {
    state.summary = emptySummary();
    state.rows = [];
    renderRanking();
    return;
  }
  state.candidates = [];
  renderCandidates();
}

async function refreshReport() {
  if (state.controller) state.controller.abort();
  const controller = new AbortController();
  const requestID = ++state.requestID;
  state.controller = controller;
  const button = $('#refresh');
  button.disabled = true;
  button.classList.add('loading');
  button.textContent = '加载中';
  $('main').setAttribute('aria-busy', 'true');
  $('main').classList.add('is-loading');
  showStatus('');
  if (state.view === 'ranking') showRankingLoading();

  try {
    if (state.view === 'overview') await loadOverview(controller.signal, requestID);
    else if (state.view === 'ranking') await loadRanking(controller.signal, requestID);
    else await loadCandidates(controller.signal, requestID);
  } catch (error) {
    if (requestID !== state.requestID || error.name === 'AbortError') return;
    clearCurrentView();
    showStatus(`报表加载失败：${error.message || '未知错误'}`);
  } finally {
    if (requestID === state.requestID) {
      button.disabled = false;
      button.classList.remove('loading');
      button.textContent = '刷新';
      $('main').removeAttribute('aria-busy');
      $('main').classList.remove('is-loading');
    }
  }
}

function setSearchContext(context) {
  state.searches[state.searchContext] = $('#search').value;
  state.searchContext = context;
  $('#search').value = state.searches[context] || '';
}

function updateSearchPrompt() {
  if (state.view === 'candidates') {
    $('#search-label').textContent = '筛选候选域名';
    $('#search').placeholder = '输入候选域名';
    return;
  }
  if (state.view === 'overview') return;
  const label = dimensionLabels[$('#dimension').value] || '对象';
  $('#search-label').textContent = `筛选${label}`;
  $('#search').placeholder = `输入${label}`;
}

function updateRankingTabLabel() {
  $('#ranking-tab').textContent = dimensionTabLabels[$('#dimension').value] || '域名流量';
}

function switchView(view) {
  cancelScheduledSearch();
  const searchContext = view === 'candidates' ? 'candidates' : view === 'ranking' ? $('#dimension').value : 'overview';
  setSearchContext(searchContext);
  state.view = view;
  $$('.report-tab').forEach((button) => {
    const active = button.dataset.view === view;
    button.classList.toggle('active', active);
    button.setAttribute('aria-selected', String(active));
  });
  $('#overview-view').classList.toggle('hidden', view !== 'overview');
  $('#ranking-view').classList.toggle('hidden', view !== 'ranking');
  $('#candidates-view').classList.toggle('hidden', view !== 'candidates');
  $('#dimension-control').classList.toggle('hidden', view !== 'ranking');
  $('#route-control').classList.toggle('hidden', view === 'candidates');
  $('#search-control').classList.toggle('hidden', view === 'overview');
  $('#filter-panel').classList.toggle('overview-mode', view === 'overview');
  $('#filter-panel').classList.toggle('ranking-mode', view === 'ranking');
  $('#filter-panel').classList.toggle('candidate-mode', view === 'candidates');
  if (view === 'ranking') {
    syncSortingForRoute();
    renderSortControls();
  }
  updateSearchPrompt();
  refreshReport();
}

function setTrendMode(mode) {
  state.trendMode = mode;
  $$('.trend-mode').forEach((button) => {
    const active = button.dataset.trendMode === mode;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  renderTrend();
}

function openProxyRanking(dimension) {
  cancelScheduledSearch();
  $('#dimension').value = dimension;
  $('#route').value = 'proxy';
  state.sort = 'proxy';
  state.order = 'desc';
  updateRankingTabLabel();
  switchView('ranking');
}

$$('.report-tab').forEach((button) => button.addEventListener('click', () => switchView(button.dataset.view)));
$$('.trend-mode').forEach((button) => button.addEventListener('click', () => setTrendMode(button.dataset.trendMode)));
$$('.sort-button').forEach((button) => button.addEventListener('click', () => changeSorting(button.dataset.sort)));
$$('.overview-drilldown').forEach((button) => button.addEventListener('click', () => openProxyRanking(button.dataset.dimension)));
$('#minutes').addEventListener('change', () => {
  cancelScheduledSearch();
  refreshReport();
});
$('#route').addEventListener('change', () => {
  cancelScheduledSearch();
  if (state.view === 'ranking') {
    syncSortingForRoute();
    renderSortControls();
  }
  refreshReport();
});
$('#dimension').addEventListener('change', () => {
  cancelScheduledSearch();
  setSearchContext($('#dimension').value);
  updateRankingTabLabel();
  updateSearchPrompt();
  refreshReport();
});
$('#refresh').addEventListener('click', () => {
  cancelScheduledSearch();
  refreshReport();
});

let searchTimer;
function cancelScheduledSearch() {
  clearTimeout(searchTimer);
}

$('#search').addEventListener('input', () => {
  cancelScheduledSearch();
  state.searches[state.searchContext] = $('#search').value;
  searchTimer = setTimeout(refreshReport, 400);
});

async function copyText(value) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch (_) {
      // Fall through to the WebView-compatible selection method.
    }
  }
  const input = document.createElement('textarea');
  input.value = value;
  input.setAttribute('readonly', '');
  input.style.position = 'fixed';
  input.style.opacity = '0';
  document.body.appendChild(input);
  input.select();
  const copied = document.execCommand('copy');
  input.remove();
  if (!copied) throw new Error('copy failed');
}

$('#candidate-body').addEventListener('click', async (event) => {
  const button = event.target.closest('.copy-button');
  if (!button) return;
  const original = '复制规则';
  clearTimeout(button.resetTimer);
  try {
    await copyText(button.dataset.rule);
    button.textContent = '已复制';
  } catch (_) {
    button.textContent = '复制失败';
  }
  button.resetTimer = setTimeout(() => { button.textContent = original; }, 1200);
});

if ('ResizeObserver' in window) {
  let resizeFrame;
  const observer = new ResizeObserver(() => {
    cancelAnimationFrame(resizeFrame);
    resizeFrame = requestAnimationFrame(() => {
      if (state.view === 'overview' && state.points.length) renderTrend();
    });
  });
  observer.observe($('#trend-chart'));
}

updateSearchPrompt();
updateRankingTabLabel();
renderSortControls();
refreshReport();

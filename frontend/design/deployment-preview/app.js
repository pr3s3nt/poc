/* Browser-only review fixture. No API, storage, credentials or infrastructure calls. */
(() => {
  'use strict';
  const $ = (selector) => document.querySelector(selector);
  const icons = {
    web: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M7 6.5h.1M10 6.5h.1M7 13h10M7 16h6"/>',
    api: '<path d="m8 6-6 6 6 6m8-12 6 6-6 6M14 4l-4 16"/>',
    db: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0"/>',
    cache: '<path d="m12 2-9 5 9 5 9-5-9-5ZM3 12l9 5 9-5M3 17l9 5 9-5"/>',
  };
  const icon = (key) => `<svg viewBox="0 0 24 24" aria-hidden="true">${icons[key]}</svg>`;
  const baseResources = {
    frontend: { name: 'Frontend', kind: 'Web application', icon: 'web', x: 221, y: 144, subtitle: 'Nginx · cổng 80', intent: 'update', description: 'Giao diện cửa hàng, nhận lưu lượng từ Internet và chuyển các yêu cầu API đến Backend.', definition: 'web-standard', module: 'score-k8s · v1.0.0', deps: ['backend'], changes: [['Container image', 'shopfront-web:1.3.0', 'shopfront-web:1.4.0']], properties: [['Image', 'shopfront-web:1.4.0'], ['Replicas', '2'], ['CPU / RAM mỗi pod', '100m / 128Mi'], ['Cổng', '80']], file: 'deployment.yaml' },
    backend: { name: 'Backend', kind: 'API service', icon: 'api', x: 221, y: 282, subtitle: 'Go API · cổng 8080', intent: 'update', description: 'Xử lý đơn hàng và dữ liệu sản phẩm. Kết nối PostgreSQL để lưu dữ liệu và Redis để tăng tốc truy vấn.', definition: 'api-standard', module: 'score-k8s · v1.0.0', deps: ['postgres', 'redis'], changes: [['Container image', 'shopfront-api:1.3.0', 'shopfront-api:1.4.0'], ['Số replica', '1 pod', '2 pods'], ['Kết nối Redis', 'Chưa cấu hình', 'redis.outputs.host → REDIS_HOST']], properties: [['Image', 'shopfront-api:1.4.0'], ['Replicas', '2'], ['CPU / RAM mỗi pod', '250m / 256Mi'], ['Cổng', '8080']], file: 'deployment.yaml' },
    postgres: { name: 'PostgreSQL', kind: 'Database', icon: 'db', x: 62, y: 430, subtitle: 'PostgreSQL 16 · dữ liệu bền vững', intent: 'retain', description: 'Lưu đơn hàng và dữ liệu sản phẩm trên volume riêng. Trong tình huống cập nhật này, cấu hình database được giữ nguyên.', definition: 'postgres-internal', module: 'postgres-k8s · v1.0.0', deps: [], changes: [], properties: [['Phiên bản', 'PostgreSQL 16'], ['Dung lượng', '10 GiB'], ['Database', 'shopfront'], ['Quyền sở hữu', 'Orchestrator']], file: 'main.tf' },
    redis: { name: 'Redis', kind: 'Cache', icon: 'cache', x: 380, y: 430, subtitle: 'Redis 7 · cache truy vấn', intent: 'create', description: 'Cache mới cho Backend. Redis được tạo và sẵn sàng trước khi Backend nhận cấu hình kết nối mới.', definition: 'redis-internal', module: 'redis-k8s · v1.0.0', deps: [], changes: [['Tài nguyên', 'Chưa tồn tại', 'Redis 7 · 1 replica'], ['Bộ nhớ', '—', '256 MiB']], properties: [['Phiên bản', 'Redis 7'], ['Replicas', '1'], ['Bộ nhớ', '256 MiB'], ['Quyền sở hữu', 'Orchestrator']], file: 'main.tf' },
  };
  const state = { scenario: 'update', outcome: 'success', selected: 'backend', tab: 'overview', file: '', codeMode: 'diff', codeExpanded: false, phase: -1, timer: null, tick: 0, paused: false, runtime: {} };
  const intentNames = { update: 'Cập nhật', create: 'Tạo mới', retain: 'Giữ nguyên' };
  const statusNames = { pending: 'Chưa chạy', waiting: 'Chờ phụ thuộc', provisioning: 'Đang cấp phát', checking: 'Kiểm tra sẵn sàng', ready: 'Sẵn sàng', retained: 'Giữ nguyên', failed: 'Thất bại', blocked: 'Bị chặn' };
  const escape = (value) => String(value).replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
  let resources = {};
  const intent = (id) => resources[id].intent;
  const done = (id) => ['ready', 'retained'].includes(state.runtime[id]?.status);
  const elapsed = (value) => `${Math.floor(value / 60).toString().padStart(2, '0')}:${(value % 60).toString().padStart(2, '0')}`;
  function resource(name, kind, iconName, properties, deps, options = {}) {
    return { name, kind, icon: iconName, properties, deps, intent: 'create', changes: [], file: 'main.tf', definition: name.toLowerCase(), module: `${kind} · v1.0.0`, subtitle: kind, description: `${name} là một thành phần trong kế hoạch triển khai minh họa.`, duration: 3, check: 2, ...options };
  }
  function configureResources() {
    const cloud = state.scenario === 'cloud';
    resources = Object.fromEntries(Object.entries(baseResources).map(([id, item]) => [id, { ...item, deps: [...item.deps], intent: state.scenario === 'update' ? item.intent : 'create', duration: 3, check: 2, location: cloud ? 'shopfront-eks / shopfront-staging' : 'dev-cluster / shopfront-staging' }]));
    resources.cluster = resource(cloud ? 'EKS' : 'dev-cluster', cloud ? 'AWS EKS' : 'Cluster có sẵn', 'web', [['Loại', cloud ? 'Amazon EKS' : 'Kubernetes nội bộ'], ['Quyền sở hữu', cloud ? 'Orchestrator' : 'Có sẵn']], cloud ? ['vpc'] : [], { intent: cloud ? 'create' : 'retain', duration: 5, location: cloud ? 'VPC shopfront · us-east-1' : 'Hạ tầng nội bộ', description: cloud ? 'Cấp phát EKS trong VPC mới. Chỉ tạo namespace sau khi cluster và worker nodes sẵn sàng.' : 'Liên kết cluster đã có. Mô phỏng không tạo lại cluster.', file: cloud ? 'main.tf' : 'binding.json', module: cloud ? 'eks · v1.0.0' : 'existing-cluster', definition: cloud ? 'eks-cloud' : 'internal-cluster' });
    resources.namespace = resource('Namespace', 'Kubernetes namespace', 'web', [['Tên', 'shopfront-staging']], ['cluster'], { intent: state.scenario === 'update' ? 'retain' : 'create', duration: 2, check: 1, location: cloud ? 'EKS shopfront-eks' : 'dev-cluster', module: 'k8s-namespace · v1.0.0', definition: 'namespace-standard', description: 'Phạm vi của ứng dụng trong cluster. Workload chờ namespace sẵn sàng trước khi được triển khai.' });
    resources.backend.deps = ['namespace', 'postgres', 'redis'];
    resources.frontend.deps = ['namespace', 'backend'];
    resources.postgres.deps = ['namespace'];
    resources.redis.deps = ['namespace'];
    resources.redis.duration = 5;
    resources.frontend.duration = 2;
    if (cloud) {
      resources.vpc = resource('VPC', 'AWS networking', 'api', [['CIDR', '10.20.0.0/16'], ['Vùng', 'us-east-1'], ['Availability Zones', '2']], [], { check: 1, location: 'AWS demo-account · us-east-1', module: 'vpc · v1.0.0', definition: 'vpc-cloud', description: 'Mạng riêng cho EKS, Aurora và ElastiCache. Sau khi mạng sẵn sàng, ba nhánh có thể được cấp phát song song.' });
      resources.postgres = resource('Aurora', 'Aurora PostgreSQL', 'db', [['Engine', 'Aurora PostgreSQL'], ['Database', 'shopfront'], ['Truy cập', 'Private subnets']], ['vpc'], { duration: 4, location: 'VPC shopfront · ngoài EKS', module: 'aurora · v1.0.0', definition: 'postgres-cloud', description: 'Database được AWS quản lý trong private subnets, nằm ngoài EKS. Backend chờ database kiểm tra sẵn sàng hoàn tất.' });
      resources.redis = resource('ElastiCache', 'Managed Redis', 'cache', [['Engine', 'Redis'], ['Node type', 'cache.t4g.micro'], ['Truy cập', 'Private subnets']], ['vpc'], { duration: 7, location: 'VPC shopfront · ngoài EKS', module: 'elasticache · v1.0.0', definition: 'redis-cloud', description: 'Cache được AWS quản lý, nằm ngoài EKS. Cấp phát song song với EKS và Aurora khi VPC sẵn sàng.' });
      const positions = { vpc:[221,50], cluster:[72,230], namespace:[72,370], backend:[72,510], frontend:[72,650], postgres:[414,230], redis:[414,430] };
      Object.entries(positions).forEach(([id,[x,y]]) => Object.assign(resources[id],{x,y}));
    } else {
      const positions = {cluster:[221,45],namespace:[221,185],postgres:[62,330],redis:[380,330],backend:[221,490],frontend:[221,635]};
      Object.entries(positions).forEach(([id,[x,y]]) => Object.assign(resources[id],{x,y}));
    }
    for (const item of Object.values(resources)) item.subtitle = item.kind;
  }
  function intentBadge(id) {
    return `<span class="badge ${intent(id)}">${intent(id) === 'create' ? '+' : intent(id) === 'update' ? '↗' : '−'} ${intentNames[intent(id)]}</span>`;
  }
  function badge(id) {
    const status = state.runtime[id]?.status ?? 'pending';
    const symbols = { pending: '○', waiting: '◷', provisioning: '◌', checking: '◉', ready: '✓', retained: '−', failed: '!', blocked: '!' };
    return `<span class="badge runtime-${status}">${symbols[status]} ${statusNames[status]}</span>`;
  }
  function changes(id) {
    if (intent(id) === 'create') return resources[id].properties.map(([key, value]) => [key, 'Chưa tồn tại', value]);
    return resources[id].changes;
  }
  function drawGraph() {
    const cloud = state.scenario === 'cloud';
    $('#graph-stage').classList.toggle('cloud-graph', cloud);
    $('#graph-background').innerHTML = cloud
      ? '<div class="cloud-boundary"><strong>AWS · demo-account · us-east-1</strong><span>VPC shopfront · mạng riêng</span></div><div class="eks-boundary"><strong>EKS shopfront-eks</strong><span>Namespace shopfront-staging</span></div><div class="managed-label">DỊCH VỤ AWS · NGOÀI EKS</div>'
      : '<div class="internal-boundary"><strong>Cluster dev-cluster</strong><span>Namespace shopfront-staging</span></div>';
    $('.map-badge').textContent = cloud ? 'AWS' : 'Kubernetes';
    const paths = cloud ? [
      'M340 150 V185 H191 V230', 'M340 150 V185 H533 V230', 'M340 150 V175 H665 V410 H533 V430',
      'M191 330 V370','M191 470 V510','M414 280 H365 V545 H310','M414 480 H380 V570 H310','M191 610 V650'
    ] : [
      'M340 145 V185','M340 285 V305 H181 V330','M340 285 V305 H499 V330','M181 430 V460 H310 V490','M499 430 V460 H370 V490','M340 590 V635'
    ];
    $('#connections').setAttribute('viewBox','0 0 680 790');
    $('#connections').innerHTML = '<defs><marker id="arrow" markerWidth="7" markerHeight="7" refX="5" refY="3.5" orient="auto"><path d="M0 0 L6 3.5 L0 7" fill="none" stroke="#8c959f" stroke-width="1.3"/></marker></defs>' + paths.map(path=>`<path d="${path}"/>`).join('');
  }
  function renderNodes() {
    const focused = document.activeElement?.dataset.node;
    $('#nodes').innerHTML = Object.entries(resources).map(([id, item]) => {
      const runtime = state.runtime[id];
      return `<button class="node ${state.selected === id ? 'selected' : ''}" data-node="${id}" data-status="${runtime.status}" style="left:${item.x}px;top:${item.y}px" aria-pressed="${state.selected === id}" aria-label="${item.name} · ${statusNames[runtime.status]}"><span class="node-icon">${icon(item.icon)}</span><span class="node-title">${item.name}</span><span class="node-sub">${item.subtitle}</span><span class="node-bottom">${badge(id)}<span>${runtime.start === null ? intentNames[intent(id)] : elapsed((runtime.end ?? state.tick) - runtime.start)}</span></span></button>`;
    }).join('');
    document.querySelectorAll('[data-node]').forEach((button) => button.addEventListener('click', () => select(button.dataset.node)));
    if (focused) document.querySelector(`[data-node="${focused}"]`)?.focus();
  }
  function select(id) {
    state.selected = id;
    state.file = '';
    renderNodes();
    renderDetail();
  }
  function runtimeDetail(id) {
    const item = resources[id], run = state.runtime[id];
    const waiting = item.deps.filter(dep => !done(dep));
    const messages = {
      pending: 'Chưa bắt đầu mô phỏng.', retained: 'Tái sử dụng tài nguyên hiện có; không cấp phát lại.',
      waiting: `Chờ ${waiting.map(dep=>resources[dep].name).join(', ')} sẵn sàng.`,
      provisioning: ['backend','frontend'].includes(id) ? 'Đang apply manifest và khởi tạo pods.' : 'Đang cấp phát tài nguyên theo cấu hình đã chọn.',
      checking: id === 'cluster' ? 'Đã cấp phát; chờ cluster và worker nodes sẵn sàng.' : 'Đã cấp phát; đang kiểm tra readiness trước khi cho thành phần phụ thuộc chạy.',
      ready: 'Đã hoàn tất cấp phát và kiểm tra sẵn sàng trong mô phỏng.',
      failed: 'READINESS_TIMEOUT · Cache chưa sẵn sàng trong thời hạn minh họa. Chưa có thao tác cleanup.',
      blocked: `Không thể tiếp tục vì phụ thuộc chưa thành công: ${waiting.map(dep=>resources[dep].name).join(', ')}.`
    };
    return `<div class="runtime-card ${run.status}"><div class="runtime-title"><strong>Trạng thái cấp phát</strong>${badge(id)}</div><p>${messages[run.status]}</p><dl><div><dt>Thời gian mô phỏng</dt><dd>${run.start === null ? 'Chưa bắt đầu' : elapsed((run.end ?? state.tick)-run.start)}</dd></div><div><dt>Bước hiện tại</dt><dd>${statusNames[run.status]}</dd></div></dl></div>`;
  }
  function overview(id) {
    const item = resources[id];
    const description = id === 'postgres' && state.scenario === 'create' ? 'Database mới lưu dữ liệu trên volume riêng trong cluster có sẵn.' : item.description;
    return `<div id="runtime-detail">${runtimeDetail(id)}</div><p class="description">${description}</p><div class="section-label">CẤU HÌNH DỰ KIẾN</div><dl class="properties">${item.properties.map(([key, value]) => `<div class="property"><dt>${key}</dt><dd>${value}</dd></div>`).join('')}<div class="property"><dt>Vị trí triển khai</dt><dd>${item.location}</dd></div><div class="property"><dt>Definition</dt><dd>${item.definition}</dd></div><div class="property"><dt>Thay đổi dự kiến</dt><dd>${intentBadge(id)}</dd></div></dl><div class="section-label">ĐIỀU KIỆN SẴN SÀNG</div><div id="dependency-status">${dependencyButtons(id)}</div><button class="code-link" id="view-code">Xem ${item.file.endsWith('.tf') ? 'Terraform' : item.file.endsWith('.json') ? 'cấu hình liên kết' : 'Kubernetes manifest'} ↗</button>`;
  }
  function dependencyButtons(id) {
    return resources[id].deps.length ? resources[id].deps.map(dep=>`<button class="dependency" data-dependency="${dep}">${resources[dep].name}<small>${statusNames[state.runtime[dep].status]} →</small></button>`).join('') : '<p class="code-note">Không có phụ thuộc phải chờ.</p>';
  }
  function bindDependencies() {
    document.querySelectorAll('[data-dependency]').forEach(button=>button.addEventListener('click',()=>select(button.dataset.dependency)));
  }
  function renderChanges(id) {
    const entries = changes(id);
    return `<p class="description">${entries.length ? 'Đối chiếu cấu hình hiện tại với cấu hình dự kiến của ' + resources[id].name + '.' : 'Không có thay đổi cấu hình trong tình huống minh họa này. Tài nguyên và dữ liệu được giữ lại.'}</p>${entries.length ? entries.map(([label, before, after]) => `<article class="change-card"><h3>${label}</h3><div class="change-row before"><span>−</span><div>${escape(before)}<small>Hiện tại</small></div></div><div class="change-row after"><span>+</span><div>${escape(after)}<small>Sau triển khai</small></div></div></article>`).join('') : `<div class="callout">✓ ${resources[id].name} được giữ nguyên trong kế hoạch minh họa.</div>`}<p class="code-note">Diff minh họa từ cấu hình mong muốn; chưa phải kết quả terraform plan hay đối chiếu cluster thực tế.</p>`;
  }
  function files(id) { return resources[id].file.endsWith('.json') ? ['binding.json'] : resources[id].file.endsWith('.tf') ? ['main.tf', 'inputs.tfvars'] : ['deployment.yaml', 'service.yaml']; }
  function code(id, file) {
    const item = resources[id];
    if (file === 'binding.json') return JSON.stringify({kind:'existing-cluster',account:'internal-demo',cluster:'dev-cluster',ownership:'PREEXISTING'}, null, 2);
    if (id === 'namespace') return file === 'main.tf' ? '# Terraform minh họa\nresource "kubernetes_namespace_v1" "app" {\n  metadata {\n    name = var.namespace\n  }\n}\n' : 'namespace = "shopfront-staging"\n';
    if (state.scenario === 'cloud' && !['backend','frontend'].includes(id)) {
      const types = {vpc:'aws_vpc',cluster:'aws_eks_cluster',postgres:'aws_rds_cluster',redis:'aws_elasticache_replication_group'};
      const configs = {
        vpc: '  cidr_block = "10.20.0.0/16"',
        cluster: '  name = "shopfront-eks"\n  # role_arn và subnet_ids từ phụ thuộc',
        postgres: '  cluster_identifier = "shopfront-db"\n  engine = "aurora-postgresql"\n  database_name = "shopfront"\n  # subnet group và credentials được lược bỏ',
        redis: '  replication_group_id = "shopfront-cache"\n  description = "Shopfront cache"\n  node_type = "cache.t4g.micro"\n  # subnet group và security groups từ VPC'
      };
      return file === 'inputs.tfvars' ? `# Inputs minh họa\nregion = "us-east-1"\nname = "shopfront-${id}"\n# Output chưa có: chờ sau provision\n` : `# Terraform minh họa · lược gọn, không dùng apply\n# Module: ${item.module}\nresource "${types[id]}" "this" {\n${configs[id]}\n}\n`;
    }
    if (file === 'inputs.tfvars') return `# Inputs minh họa · không chứa credentials\nnamespace = "shopfront-staging"\nname      = "${id}"\n${id === 'postgres' ? 'database  = "shopfront"\nstorage   = "10Gi"\n# password: lấy từ Secret Store\n# Giá trị bí mật không được hiển thị' : 'memory    = "256Mi"\nreplicas  = 1'}\n`;
    if (file === 'main.tf') return `# Mã ví dụ cho review UI\n# Module: ${item.module}\n# Chưa đọc source của module thực tế\n\nresource "kubernetes_stateful_set_v1" "${id}" {\n  metadata {\n    name      = var.name\n    namespace = var.namespace\n  }\n  spec {\n    service_name = var.name\n    replicas     = 1\n    selector {\n      match_labels = { app = var.name }\n    }\n    template {\n      metadata {\n        labels = { app = var.name }\n      }\n      spec {\n        container {\n          name  = "${id}"\n          image = "${id === 'postgres' ? 'postgres:16' : 'redis:7'}"\n          # Các trường còn lại được lược gọn\n        }\n      }\n    }\n  }\n}\n`;
    if (file === 'service.yaml') return `# Manifest minh họa\napiVersion: v1\nkind: Service\nmetadata:\n  name: ${id}\n  namespace: shopfront-staging\nspec:\n  selector:\n    app: ${id}\n  ports:\n    - port: ${id === 'backend' ? 8080 : 80}\n      targetPort: ${id === 'backend' ? 8080 : 80}\n`;
    return `# Manifest dự kiến · dữ liệu minh họa\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: ${id}\n  namespace: shopfront-staging\nspec:\n  replicas: 2\n  selector:\n    matchLabels:\n      app: ${id}\n  template:\n    metadata:\n      labels:\n        app: ${id}\n    spec:\n      containers:\n        - name: ${id}\n          image: ${item.properties[0][1]}\n          ports:\n            - containerPort: ${id === 'backend' ? 8080 : 80}\n          resources:\n            requests:\n              cpu: ${id === 'backend' ? '250m' : '100m'}\n              memory: ${id === 'backend' ? '256Mi' : '128Mi'}\n${id === 'backend' ? '          env:\n            - name: REDIS_HOST\n              # Chờ redis.outputs.host\n              value: "<có sau provision>"\n            - name: DB_PASSWORD\n              valueFrom:\n                secretKeyRef:\n                  name: postgres-credentials\n                  key: password\n' : ''}`;
  }
  function highlight(value) {
    return value.split('\n').map((line) => {
      if (line.trimStart().startsWith('#')) return `<span class="comment">${escape(line)}</span>`;
      const match = line.match(/^(\s*)([a-zA-Z_][a-zA-Z0-9_]*)(:|\s*=)(.*)$/);
      if (match) return `${match[1]}<span class="key">${match[2]}</span>${match[3]}<span class="value">${escape(match[4])}</span>`;
      return escape(line);
    }).join('\n');
  }
  function beforeCode(id, file) {
    if (intent(id) === 'create') return '';
    const after = code(id, file);
    if (intent(id) === 'retain' || file !== 'deployment.yaml') return after;
    let before = after.replace(':1.4.0', ':1.3.0');
    if (id === 'backend') {
      before = before.replace('replicas: 2', 'replicas: 1')
        .replace('            - name: REDIS_HOST\n              # Chờ redis.outputs.host\n              value: "<có sau provision>"\n', '');
    }
    return before;
  }
  // Compare the two fixture documents, so unchanged lines never inherit a resource-level change color.
  function diffLines(before, after) {
    const lines = (text) => text === '' ? [] : text.replace(/\n$/, '').split('\n');
    const oldLines = lines(before), newLines = lines(after);
    const lengths = Array.from({ length: oldLines.length + 1 }, () => new Uint16Array(newLines.length + 1));
    for (let i = oldLines.length - 1; i >= 0; i -= 1) {
      for (let j = newLines.length - 1; j >= 0; j -= 1) {
        lengths[i][j] = oldLines[i] === newLines[j] ? lengths[i + 1][j + 1] + 1 : Math.max(lengths[i + 1][j], lengths[i][j + 1]);
      }
    }
    const result = [];
    let i = 0, j = 0;
    while (i < oldLines.length || j < newLines.length) {
      if (i < oldLines.length && j < newLines.length && oldLines[i] === newLines[j]) {
        result.push({ kind: 'context', old: i + 1, next: j + 1, text: oldLines[i] }); i += 1; j += 1;
      } else if (i < oldLines.length && (j === newLines.length || lengths[i + 1][j] >= lengths[i][j + 1])) {
        result.push({ kind: 'removed', old: i + 1, next: '', text: oldLines[i] }); i += 1;
      } else {
        result.push({ kind: 'added', old: '', next: j + 1, text: newLines[j] }); j += 1;
      }
    }
    return result;
  }
  function renderCode(id) {
    const available = files(id);
    if (!available.includes(state.file)) state.file = available[0];
    const after = code(id, state.file);
    const diff = diffLines(beforeCode(id, state.file), after);
    const added = diff.filter((line) => line.kind === 'added').length;
    const removed = diff.filter((line) => line.kind === 'removed').length;
    const isDiff = state.codeMode === 'diff';
    const rows = diff.map((line) => `<span class="diff-line ${line.kind}"><span class="line-number" aria-hidden="true">${line.old}</span><span class="line-number" aria-hidden="true">${line.next}</span><span class="diff-sign" aria-label="${line.kind === 'added' ? 'Dòng thêm' : line.kind === 'removed' ? 'Dòng bỏ' : 'Giữ nguyên'}">${line.kind === 'added' ? '+' : line.kind === 'removed' ? '−' : ' '}</span><span class="line-code">${highlight(line.text) || ' '}</span></span>`).join('');
    return `<div class="file-buttons">${available.map((file) => `<button data-file="${file}" aria-pressed="${file === state.file}" class="${file === state.file ? 'selected' : ''}">${file}</button>`).join('')}</div>
      <div class="code-toolbar"><div class="code-modes" role="group" aria-label="Chế độ xem code"><button data-code-mode="diff" aria-pressed="${isDiff}">Xem diff</button><button data-code-mode="after" aria-pressed="${!isDiff}">Sau thay đổi</button></div><button id="expand-code" aria-expanded="${state.codeExpanded}">${state.codeExpanded ? 'Thu gọn' : 'Mở rộng'}</button></div>
      <div class="diff-summary" role="status">${added || removed ? `<span class="added-count">+${added} dòng thêm</span><span class="removed-count">−${removed} dòng bỏ</span>` : 'Không có thay đổi trong file này.'}</div>
      ${isDiff ? '<p class="diff-hint">Số dòng: trước / sau · Đỏ: bỏ · Xanh: thêm</p>' : ''}
      <div class="code-header"><span>${state.file}</span><button id="copy-code" title="Sao chép nội dung sau thay đổi, không kèm ký hiệu diff">Sao chép bản sau</button></div>
      <pre class="code-block ${isDiff ? 'diff-block' : ''}"><code>${isDiff ? rows : highlight(after)}</code></pre>
      <p class="code-note">${resources[id].module}<br>${state.file.endsWith('.yaml') ? 'Manifest minh họa, chưa render từ deployment thật. Placeholder thể hiện giá trị chờ provision.' : 'Đoạn Terraform minh họa được lược gọn để review giao diện; không phải source module hoặc plan thực tế.'}</p>`;
  }
  function renderDetail() {
    const id = state.selected;
    const item = resources[id];
    $('.review-layout').classList.toggle('code-expanded', state.tab === 'code' && state.codeExpanded);
    $('#detail-heading').innerHTML = `<div class="detail-heading"><div class="detail-icon">${icon(item.icon)}</div><div><h2>${item.name}</h2><p>${item.kind} · staging</p></div>${badge(id)}</div>`;
    $('#change-count').textContent = changes(id).length;
    document.querySelectorAll('[data-tab]').forEach((button) => { button.setAttribute('aria-selected', String(button.dataset.tab === state.tab)); button.tabIndex = button.dataset.tab === state.tab ? 0 : -1; });
    $('#detail-body').setAttribute('aria-label', `${item.name} · ${state.tab === 'overview' ? 'Tổng quan' : state.tab === 'changes' ? 'Thay đổi' : 'Code'}`);
    $('#detail-body').innerHTML = state.tab === 'overview' ? overview(id) : state.tab === 'changes' ? renderChanges(id) : renderCode(id);
    bindDependencies();
    $('#view-code')?.addEventListener('click', () => { state.tab = 'code'; renderDetail(); document.querySelector('[data-tab="code"]').focus(); });
    document.querySelectorAll('[data-file]').forEach((button) => button.addEventListener('click', () => { state.file = button.dataset.file; renderDetail(); document.querySelector(`[data-file="${state.file}"]`).focus(); }));
    document.querySelectorAll('[data-code-mode]').forEach((button) => button.addEventListener('click', () => { state.codeMode = button.dataset.codeMode; renderDetail(); document.querySelector(`[data-code-mode="${state.codeMode}"]`).focus(); }));
    $('#expand-code')?.addEventListener('click', () => { state.codeExpanded = !state.codeExpanded; renderDetail(); $('#expand-code').focus(); });
    $('#copy-code')?.addEventListener('click', async () => {
      const button = $('#copy-code');
      try { await navigator.clipboard.writeText(code(state.selected, state.file)); button.textContent = 'Đã sao chép ✓'; }
      catch {
        // Expose clean after-code for manual copying, never include diff gutters or removed lines.
        state.codeMode = 'after'; renderDetail();
        $('#copy-code').textContent = 'Code đã chọn · Ctrl+C';
        const selection = window.getSelection(); const range = document.createRange();
        range.selectNodeContents($('.code-block')); selection.removeAllRanges(); selection.addRange(range);
      }
    });
  }
  function renderSteps() {
    const entries = Object.entries(state.runtime);
    const complete = entries.filter(([,run])=>['ready','retained'].includes(run.status)).length;
    const active = entries.filter(([,run])=>['provisioning','checking'].includes(run.status));
    const failed = entries.some(([,run])=>run.status==='failed');
    $('#progress-summary').innerHTML = `<div class="progress-track"><div style="width:${complete / entries.length * 100}%"></div></div><div class="progress-copy"><strong>${complete}/${entries.length} thành phần sẵn sàng hoặc giữ nguyên</strong><span>${active.length} đang xử lý${failed ? ' · Có lỗi' : ''}</span></div>`;
    $('#steps').innerHTML = entries.filter(([,run])=>['provisioning','checking','failed','blocked'].includes(run.status)).map(([id,run])=>`<li class="runtime-${run.status}"><span class="step-num">${run.status==='failed'?'!':'◌'}</span><div class="step-text"><strong>${resources[id].name}</strong><small>${statusNames[run.status]}</small></div></li>`).join('') || '<li class="step-text"><small>Tiến độ từng thành phần được hiển thị trực tiếp trên sơ đồ.</small></li>';
    const status = state.phase === -1 ? 'Chưa chạy mô phỏng' : state.phase === 1 ? (failed ? 'Mô phỏng kết thúc · có lỗi' : '✓ Hoàn tất mô phỏng') : `${state.paused ? 'Tạm dừng' : 'Đang mô phỏng'} · ${elapsed(state.tick)}`;
    $('#execution-status').textContent = status;
    $('#overall-status').textContent = state.phase === -1 ? 'Chờ mô phỏng' : state.phase === 1 ? (failed?'Mô phỏng có lỗi':'Mô phỏng hoàn tất') : state.paused?'Mô phỏng tạm dừng':'Đang mô phỏng';
    $('#overall-status').style.color = failed ? 'var(--red)' : state.phase === 1 ? 'var(--green)' : 'var(--yellow)';
    $('#simulate').disabled = state.phase === 0;
    $('#simulate').textContent = state.phase === 1 ? '↻ Mô phỏng lại' : '▷ Mô phỏng triển khai';
    $('#pause').disabled = state.phase !== 0;
    $('#pause').textContent = state.paused ? 'Tiếp tục' : 'Tạm dừng';
    $('#outcome').disabled = state.phase === 0;
    $('#footer-title').textContent = status;
    $('#footer-description').textContent = failed ? 'Phụ thuộc bị chặn; tài nguyên đã tạo vẫn được giữ. Đây là mô phỏng.' : 'Trạng thái và thời gian là minh họa. Không tạo hạ tầng thật.';
  }
  function render() {
    const counts = {create:0,update:0,retain:0};
    Object.values(resources).forEach(item=>counts[item.intent]++);
    $('#counts').innerHTML = Object.entries(counts).filter(([,n])=>n).map(([key,n])=>`<span><i class="dot ${key==='create'?'green':key==='update'?'orange':'gray'}"></i><b>${n}</b> ${intentNames[key].toLowerCase()}</span>`).join('');
    $('#summary-title').textContent = `${Object.keys(resources).length} thành phần trong kế hoạch`;
    $('#summary-sub').textContent = state.scenario==='cloud' ? 'AWS demo-account · us-east-1 · VPC và EKS mới' : 'Cluster có sẵn · namespace shopfront-staging';
    $('#execution-description').textContent = state.scenario==='cloud' ? 'VPC sẵn sàng → EKS, Aurora và ElastiCache chạy song song → namespace và ứng dụng.' : 'Cluster có sẵn → namespace → database/cache → Backend → Frontend.';
    drawGraph(); renderNodes(); renderDetail(); renderSteps();
  }
  function reset() {
    clearTimeout(state.timer); state.timer=null; state.phase=-1; state.tick=0; state.paused=false;
    state.runtime=Object.fromEntries(Object.keys(resources).map(id=>[id,{status:intent(id)==='retain'?'retained':'pending',start:null,end:null,checkingAt:null}]));
    render();
  }
  function refreshRuntime() {
    renderNodes(); renderSteps();
    $('#detail-heading .badge').outerHTML=badge(state.selected);
    if ($('#runtime-detail')) $('#runtime-detail').innerHTML=runtimeDetail(state.selected);
    if ($('#dependency-status')) { $('#dependency-status').innerHTML=dependencyButtons(state.selected); bindDependencies(); }
  }
  function advance() {
    // Finish active nodes before admitting their consumers. Providers may run in parallel.
    for (const [id,run] of Object.entries(state.runtime)) {
      if (run.status==='provisioning' && state.tick-run.start>=resources[id].duration) {run.status='checking';run.checkingAt=state.tick;}
      else if(run.status==='checking' && state.tick-run.checkingAt>=resources[id].check) {run.status=state.outcome==='failure'&&id==='redis'?'failed':'ready';run.end=state.tick;}
    }
    // Propagate failure through the whole dependency chain before scheduling work.
    for(let pass=0;pass<Object.keys(resources).length;pass++) {
      for(const [id,run] of Object.entries(state.runtime)) {
        if(['pending','waiting'].includes(run.status) && resources[id].deps.some(dep=>['failed','blocked'].includes(state.runtime[dep].status))) run.status='blocked';
      }
    }
    for(const [id,run] of Object.entries(state.runtime)) {
      if(!['pending','waiting'].includes(run.status)) continue;
      if(resources[id].deps.every(done)) {run.status='provisioning';run.start=state.tick;} else run.status='waiting';
    }
    if(Object.values(state.runtime).every(run=>['ready','retained','failed','blocked'].includes(run.status))) state.phase=1;
    refreshRuntime();
    if(state.phase===0&&!state.paused) state.timer=setTimeout(()=>{state.tick++;advance();},1000);
  }
  $('#scenario').addEventListener('change', event=>{state.scenario=event.target.value;state.file='';configureResources();if(!resources[state.selected])state.selected='backend';reset();});
  $('#outcome').addEventListener('change', event=>{state.outcome=event.target.value;reset();});
  $('#reset').addEventListener('click',reset);
  $('#simulate').addEventListener('click',()=>{reset();state.phase=0;advance();});
  $('#pause').addEventListener('click',()=>{state.paused=!state.paused;clearTimeout(state.timer);if(!state.paused)state.timer=setTimeout(()=>{state.tick++;advance();},1000);renderSteps();});
  document.querySelectorAll('[data-tab]').forEach((button, index, tabs) => {
    button.addEventListener('click', () => { state.tab = button.dataset.tab; renderDetail(); });
    button.addEventListener('keydown', (event) => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault();
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next].click(); tabs[next].focus();
    });
  });
  configureResources(); reset();
})();

(function () {
  'use strict';

  const API = '/apis/dcm.io/v1alpha1';
  const $ = (sel, ctx = document) => ctx.querySelector(sel);
  const $$ = (sel, ctx = document) => [...ctx.querySelectorAll(sel)];

  // --- API Client ---
  async function api(resource) {
    const resp = await fetch(`${API}/${resource}`);
    if (!resp.ok) return { items: [] };
    return resp.json();
  }

  async function apiGet(resource, name) {
    const resp = await fetch(`${API}/${resource}/${name}`);
    if (!resp.ok) return null;
    const data = await resp.json();
    data._revision = resp.headers.get('X-DCM-Revision');
    return data;
  }

  async function apiCreate(resource, yamlBody) {
    const resp = await fetch(`${API}/${resource}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/yaml' },
      body: yamlBody,
    });
    const body = await resp.text();
    if (!resp.ok) {
      let msg = body;
      try { msg = JSON.parse(body).error; } catch(_) {}
      throw new Error(msg);
    }
    return { revision: resp.headers.get('X-DCM-Revision') };
  }

  async function apiDelete(resource, name, revision) {
    const resp = await fetch(`${API}/${resource}/${name}`, {
      method: 'DELETE',
      headers: { 'X-DCM-Revision': String(revision) },
    });
    if (!resp.ok) {
      const body = await resp.text();
      let msg = body;
      try { msg = JSON.parse(body).error; } catch(_) {}
      throw new Error(msg);
    }
  }

  async function apiUpdate(resource, name, yamlBody, revision) {
    const resp = await fetch(`${API}/${resource}/${name}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/yaml', 'X-DCM-Revision': String(revision) },
      body: yamlBody,
    });
    const body = await resp.text();
    if (!resp.ok) {
      let msg = body;
      try { msg = JSON.parse(body).error; } catch(_) {}
      throw new Error(msg);
    }
    return { revision: resp.headers.get('X-DCM-Revision') };
  }

  // --- Modal ---

  function showCreateModal(title, yamlTemplate, resource, onSuccess) {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
      <div class="modal">
        <div class="modal-header">
          <h3>${esc(title)}</h3>
          <button class="modal-close">&times;</button>
        </div>
        <div class="modal-body">
          <textarea class="modal-editor" spellcheck="false">${esc(yamlTemplate.trim())}</textarea>
          <div class="modal-error" style="display:none"></div>
        </div>
        <div class="modal-footer">
          <button class="modal-btn modal-btn-cancel">Cancel</button>
          <button class="modal-btn modal-btn-create">Create</button>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);

    const close = () => overlay.remove();
    overlay.querySelector('.modal-close').onclick = close;
    overlay.querySelector('.modal-btn-cancel').onclick = close;
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });

    const editor = overlay.querySelector('.modal-editor');
    const errorEl = overlay.querySelector('.modal-error');
    const createBtn = overlay.querySelector('.modal-btn-create');

    createBtn.onclick = async () => {
      errorEl.style.display = 'none';
      createBtn.disabled = true;
      createBtn.textContent = 'Creating...';
      try {
        await apiCreate(resource, editor.value);
        close();
        if (onSuccess) onSuccess();
      } catch (err) {
        errorEl.textContent = err.message;
        errorEl.style.display = 'block';
        createBtn.disabled = false;
        createBtn.textContent = 'Create';
      }
    };

    // Focus and select the name field
    setTimeout(() => editor.focus(), 100);
  }

  function showDeleteConfirm(kindLabel, name, resource, revision, redirectHash) {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
      <div class="modal" style="width:440px">
        <div class="modal-header">
          <h3>Delete ${esc(kindLabel)}</h3>
          <button class="modal-close">&times;</button>
        </div>
        <div class="modal-body">
          <p style="margin-bottom:12px">Are you sure you want to delete <strong>${esc(name)}</strong>?</p>
          <p style="color:var(--text-muted);font-size:13px">This action cannot be undone.</p>
          <div class="modal-error" style="display:none"></div>
        </div>
        <div class="modal-footer">
          <button class="modal-btn modal-btn-cancel">Cancel</button>
          <button class="modal-btn modal-btn-delete">Delete</button>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);

    const close = () => overlay.remove();
    overlay.querySelector('.modal-close').onclick = close;
    overlay.querySelector('.modal-btn-cancel').onclick = close;
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });

    const errorEl = overlay.querySelector('.modal-error');
    const deleteBtn = overlay.querySelector('.modal-btn-delete');

    deleteBtn.onclick = async () => {
      errorEl.style.display = 'none';
      deleteBtn.disabled = true;
      deleteBtn.textContent = 'Deleting...';
      try {
        await apiDelete(resource, name, revision);
        close();
        location.hash = redirectHash;
        navigate();
      } catch (err) {
        errorEl.textContent = err.message;
        errorEl.style.display = 'block';
        deleteBtn.disabled = false;
        deleteBtn.textContent = 'Delete';
      }
    };
  }

  function showEditModal(title, resource, name, revision, onSuccess) {
    // Fetch current YAML
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
      <div class="modal">
        <div class="modal-header">
          <h3>${esc(title)}</h3>
          <button class="modal-close">&times;</button>
        </div>
        <div class="modal-body">
          <div class="loading"><div class="spinner"></div>Loading...</div>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);

    const close = () => overlay.remove();
    overlay.querySelector('.modal-close').onclick = close;
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close(); });

    // Fetch current object as YAML
    fetch(`${API}/${resource}/${name}`, { headers: { 'Accept': 'application/yaml' } })
      .then(resp => {
        revision = resp.headers.get('X-DCM-Revision') || revision;
        return resp.text();
      })
      .then(yaml => {
        const body = overlay.querySelector('.modal-body');
        body.innerHTML = `
          <textarea class="modal-editor" spellcheck="false">${esc(yaml)}</textarea>
          <div class="modal-error" style="display:none"></div>
        `;

        // Add footer
        const footer = document.createElement('div');
        footer.className = 'modal-footer';
        footer.innerHTML = `
          <button class="modal-btn modal-btn-cancel">Cancel</button>
          <button class="modal-btn modal-btn-create">Save</button>
        `;
        overlay.querySelector('.modal').appendChild(footer);

        footer.querySelector('.modal-btn-cancel').onclick = close;
        const editor = body.querySelector('.modal-editor');
        const errorEl = body.querySelector('.modal-error');
        const saveBtn = footer.querySelector('.modal-btn-create');

        saveBtn.onclick = async () => {
          errorEl.style.display = 'none';
          saveBtn.disabled = true;
          saveBtn.textContent = 'Saving...';
          try {
            await apiUpdate(resource, name, editor.value, revision);
            close();
            if (onSuccess) onSuccess();
          } catch (err) {
            errorEl.textContent = err.message;
            errorEl.style.display = 'block';
            saveBtn.disabled = false;
            saveBtn.textContent = 'Save';
          }
        };

        setTimeout(() => editor.focus(), 100);
      });
  }

  // --- Router ---
  const routes = {
    '/': renderOverview,
    '/applications': renderApplications,
    '/applications/:name': renderApplicationDetail,
    '/environments': renderEnvironments,
    '/environments/:name': renderEnvironmentDetail,
    '/resourcetypes': renderResourceTypes,
    '/resourcetypes/:name': renderResourceTypeDetail,
    '/recipes': renderRecipes,
    '/deployments': renderDeployments,
    '/deployments/:name': renderDeploymentDetail,
    '/placement': renderPlacement,
    '/policies': renderPolicies,
  };

  function navigate() {
    const hash = location.hash.slice(1) || '/';
    const content = $('#page-content');

    // Update active nav
    $$('.nav-link').forEach(link => {
      const page = link.dataset.page;
      const isActive = hash === '/' ? page === 'overview' :
        hash.startsWith('/' + page);
      link.classList.toggle('active', isActive);
    });

    // Match route
    for (const [pattern, handler] of Object.entries(routes)) {
      const match = matchRoute(pattern, hash);
      if (match !== null) {
        content.innerHTML = '<div class="loading"><div class="spinner"></div>Loading...</div>';
        handler(content, match);
        return;
      }
    }

    content.innerHTML = '<div class="empty-state"><h3>Page not found</h3></div>';
  }

  function matchRoute(pattern, path) {
    const patternParts = pattern.split('/');
    const pathParts = path.split('/');
    if (patternParts.length !== pathParts.length) return null;

    const params = {};
    for (let i = 0; i < patternParts.length; i++) {
      if (patternParts[i].startsWith(':')) {
        params[patternParts[i].slice(1)] = decodeURIComponent(pathParts[i]);
      } else if (patternParts[i] !== pathParts[i]) {
        return null;
      }
    }
    return params;
  }

  window.addEventListener('hashchange', navigate);
  window.addEventListener('load', navigate);

  // --- YAML Templates ---

  const TMPL_APP = `
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: my-app
  labels:
    team: platform
spec:
  resources:
    - type: database.postgresql
      name: my-db
      properties:
        size: S
    - type: compute.container
      name: my-api
      properties:
        image: quay.io/example/api
      requirements:
        - my-db`;

  const TMPL_ENV = `
apiVersion: dcm.io/v1alpha1
kind: Environment
metadata:
  name: prod-eu-k8s
  labels:
    tier: production
spec:
  type: kubernetes
  description: "Production Kubernetes in Frankfurt"
  connection:
    endpoint: "https://k8s-prod.example.com:6443"
    credentialRef: "vault:secret/dcm/prod-eu"
  capabilities:
    resourceTypes:
      - database.postgresql
      - compute.container
    features:
      - ssd-storage
  sovereignty:
    country: DE
    region: eu-central-1
    jurisdiction: EU
    compliance:
      - GDPR
    dataClassification: confidential`;

  const TMPL_RT = `
apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: database.postgresql
  labels:
    category: database
spec:
  version: "1.0.0"
  lifecycle: stable
  schema:
    type: object
    required:
      - size
    properties:
      size:
        type: string
        description: "Instance size"
        enum: ["S", "M", "L"]
        default: "S"
      host:
        type: string
        description: "Database hostname"
        readOnly: true
      port:
        type: integer
        description: "Database port"
        readOnly: true`;

  const TMPL_RECIPE = `
apiVersion: dcm.io/v1alpha1
kind: Recipe
metadata:
  name: compute-container-ansible-k8s
  labels:
    resourceType: compute.container
spec:
  resourceType: compute.container
  resourceTypeVersion: ">=1.0.0"
  type: ansible
  source:
    playbook: "playbooks/compute.container/provision.yml"
    destroyPlaybook: "playbooks/compute.container/destroy.yml"
  environmentMatch:
    types:
      - kubernetes
      - openshift`;

  const TMPL_POLICY = `
apiVersion: dcm.io/v1alpha1
kind: PlacementPolicy
metadata:
  name: prefer-eu
spec:
  match:
    all: true
  rule: 'env.sovereignty.jurisdiction == "EU"'
  prefer: "env.capacity.cpu.total"
  weight: 1.0
  priority: 100`;

  // --- Helpers ---
  function badge(text, type = 'neutral') {
    return `<span class="badge badge-${type}">${esc(text)}</span>`;
  }

  function phaseBadge(phase) {
    const map = {
      Ready: 'success', Provisioned: 'success',
      Pending: 'neutral', Validating: 'info', Placing: 'info', Provisioning: 'info',
      Failed: 'danger',
      stable: 'success', draft: 'warning', deprecated: 'danger',
    };
    return badge(phase, map[phase] || 'neutral');
  }

  function typeBadge(type) {
    const map = {
      kubernetes: 'info', aws: 'warning', azure: 'info', gcp: 'success',
      openshift: 'danger', vmware: 'accent', 'bare-metal': 'neutral',
      terraform: 'accent', helm: 'info', ansible: 'danger',
    };
    return badge(type, map[type] || 'neutral');
  }

  function tags(labels) {
    if (!labels || Object.keys(labels).length === 0) return '<span class="text-muted">-</span>';
    return '<div class="tags">' +
      Object.entries(labels).map(([k, v]) =>
        `<span class="tag"><span class="tag-key">${esc(k)}:</span>${esc(v)}</span>`
      ).join('') + '</div>';
  }

  function esc(s) {
    if (s == null) return '';
    const el = document.createElement('span');
    el.textContent = String(s);
    return el.innerHTML;
  }

  function kvGrid(items) {
    return '<div class="kv-grid">' +
      items.map(([label, value]) =>
        `<div class="kv-item">
          <div class="kv-label">${esc(label)}</div>
          <div class="kv-value">${value}</div>
        </div>`
      ).join('') + '</div>';
  }

  function backLink(href, label) {
    return `<a class="detail-back" href="${href}">← ${esc(label)}</a>`;
  }

  // --- Pages ---

  async function renderOverview(el) {
    const [apps, envs, rts, recipes, policies, deployments] = await Promise.all([
      api('applications'), api('environments'), api('resourcetypes'),
      api('recipes'), api('placementpolicies'), api('deployments'),
    ]);

    el.innerHTML = `
      <div class="page-header">
        <h2>Overview</h2>
        <p>DCM Control Plane dashboard</p>
      </div>
      <div class="stats-grid">
        <a href="#/applications" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--accent)">${apps.items?.length || 0}</div>
          <div class="stat-label">Applications</div>
        </a>
        <a href="#/environments" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--success)">${envs.items?.length || 0}</div>
          <div class="stat-label">Environments</div>
        </a>
        <a href="#/resourcetypes" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--info)">${rts.items?.length || 0}</div>
          <div class="stat-label">Resource Types</div>
        </a>
        <a href="#/recipes" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--warning)">${recipes.items?.length || 0}</div>
          <div class="stat-label">Recipes</div>
        </a>
        <a href="#/deployments" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--warning)">${deployments.items?.length || 0}</div>
          <div class="stat-label">Deployments</div>
        </a>
        <a href="#/policies" class="stat-card" style="text-decoration:none;color:inherit">
          <div class="stat-value" style="color:var(--text-secondary)">${policies.items?.length || 0}</div>
          <div class="stat-label">Policies</div>
        </a>
      </div>
      <div class="detail-section">
        <h3>Recent Applications</h3>
        ${apps.items?.length ? renderAppCards(apps.items.slice(0, 6)) :
          '<div class="empty-state"><div class="empty-state-icon">&#128230;</div><h3>No applications yet</h3><p>Create an Application via the API to get started.</p></div>'}
      </div>
    `;
  }

  function renderAppCards(items) {
    return '<div class="cards-grid">' + items.map(app => {
      const resources = app.spec?.resources || [];
      return `
        <div class="card" onclick="location.hash='#/applications/${app.metadata.name}'">
          <div class="card-header">
            <div>
              <div class="card-title">${esc(app.metadata.name)}</div>
              <div class="card-subtitle">${resources.length} resource${resources.length !== 1 ? 's' : ''}</div>
            </div>
          </div>
          <div class="resource-list">
            ${resources.slice(0, 4).map(r => `
              <div class="resource-item">
                <span class="resource-name">${esc(r.name)}</span>
                <span class="resource-type">${esc(r.type)}</span>
              </div>
            `).join('')}
            ${resources.length > 4 ? `<div class="resource-item" style="justify-content:center;color:var(--text-muted)">+${resources.length - 4} more</div>` : ''}
          </div>
        </div>`;
    }).join('') + '</div>';
  }

  async function renderApplications(el) {
    const data = await api('applications');
    const items = data.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Applications</h2>
          <p>${items.length} application${items.length !== 1 ? 's' : ''} registered</p>
        </div>
        <button class="create-btn" id="create-app-btn">+ Create Application</button>
      </div>
      ${items.length ? renderAppCards(items) :
        '<div class="empty-state"><div class="empty-state-icon">&#128230;</div><h3>No applications</h3><p>Click "Create Application" to get started.</p></div>'}
    `;

    $('#create-app-btn', el).onclick = () =>
      showCreateModal('Create Application', TMPL_APP, 'applications', () => navigate());
  }

  async function renderApplicationDetail(el, params) {
    const app = await apiGet('applications', params.name);
    if (!app) {
      el.innerHTML = `${backLink('#/applications', 'Applications')}<div class="empty-state"><h3>Application not found</h3></div>`;
      return;
    }

    const resources = app.spec?.resources || [];
    // Build simple DAG levels for visualization
    const dagLevels = buildSimpleDAG(resources);

    el.innerHTML = `
      ${backLink('#/applications', 'Applications')}
      <div class="detail-header">
        <div>
          <div class="detail-title">${esc(app.metadata.name)}</div>
          <div class="detail-meta">${resources.length} resource${resources.length !== 1 ? 's' : ''}</div>
        </div>
        <div style="display:flex;gap:10px;align-items:center">
          <button class="create-btn" id="deploy-app-btn">Deploy</button>
          <button class="delete-btn" id="delete-app-btn">Delete</button>
        </div>
      </div>

      ${app.metadata.labels ? `<div class="detail-section"><h3>Labels</h3>${tags(app.metadata.labels)}</div>` : ''}

      <div class="detail-section">
        <h3>Application Graph</h3>
        <div class="dag-container">
          ${dagLevels.length ? renderDAG(dagLevels, resources) :
            '<div class="empty-state"><p>No resources to visualize</p></div>'}
        </div>
      </div>

      <div class="detail-section">
        <h3>Resources</h3>
        <div class="table-container">
          <table>
            <thead><tr><th>Name</th><th>Type</th><th>Requirements</th><th>Properties</th></tr></thead>
            <tbody>
              ${resources.map(r => `
                <tr>
                  <td><span class="resource-name">${esc(r.name)}</span></td>
                  <td>${badge(r.type, 'accent')}</td>
                  <td>${r.requirements?.length ? r.requirements.map(d => badge(d, 'neutral')).join(' ') : '-'}</td>
                  <td><code style="font-size:12px;color:var(--text-secondary)">${esc(JSON.stringify(r.properties || {}).substring(0, 80))}</code></td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        </div>
      </div>

      <div id="deploy-result"></div>
    `;

    $('#deploy-app-btn', el).onclick = () => runDeploy(app.metadata.name);
    $('#delete-app-btn', el).onclick = () =>
      showDeleteConfirm('Application', app.metadata.name, 'applications', app._revision, '#/applications');
  }

  async function runDeploy(appName) {
    const resultEl = document.getElementById('deploy-result');
    resultEl.innerHTML = '<div class="loading"><div class="spinner"></div>Deploying application...</div>';

    let data;
    try {
      const resp = await fetch(API + '/deploy/' + encodeURIComponent(appName), { method: 'POST' });
      data = await resp.json();
    } catch (err) {
      resultEl.innerHTML = `<div class="detail-section" style="background:var(--danger-bg);padding:16px;border-radius:var(--radius-md)">
        <strong style="color:var(--danger)">Deploy Error</strong>
        <pre style="margin-top:8px;color:var(--text-secondary);white-space:pre-wrap">${esc(err.message)}</pre>
      </div>`;
      return;
    }

    const isSuccess = data.phase === 'Provisioned';
    const assignments = data.assignments || {};
    const levels = data.levels || [];
    const resources = data.resources || [];
    const decisions = data.decisions || [];

    resultEl.innerHTML = `
      <div class="detail-section">
        <h3>Deploy Result</h3>
        <div style="background:${isSuccess ? 'var(--success-bg)' : 'var(--danger-bg)'};padding:14px;border-radius:var(--radius-md);margin-bottom:20px;border:1px solid ${isSuccess ? 'rgba(34,197,94,0.3)' : 'rgba(239,68,68,0.3)'}">
          <strong style="color:${isSuccess ? 'var(--success)' : 'var(--danger)'}">${isSuccess ? 'Deploy Succeeded' : 'Deploy Failed'}</strong>
          ${data.error ? `<pre style="margin-top:8px;color:var(--text-secondary);font-size:13px;white-space:pre-wrap">${esc(data.error)}</pre>` : ''}
        </div>

        ${levels.length ? `<div class="detail-section">
          <h3>Execution Order</h3>
          <div class="dag-container">
            <div class="dag-levels">
              ${levels.map((level, i) => {
                const nodes = '<div class="dag-level">' + level.map(name => {
                  const rs = resources.find(r => r.name === name);
                  const env = assignments[name] || '';
                  const phase = rs?.phase || 'Pending';
                  const nodeColor = phase === 'Provisioned' ? 'var(--success)' : phase === 'Failed' ? 'var(--danger)' : 'var(--border)';
                  return '<div class="dag-node" style="border-color:' + nodeColor + '">' +
                    '<div class="dag-node-name">' + esc(name) + '</div>' +
                    '<div class="dag-node-type">' + esc(env) + '</div>' +
                    phaseBadge(phase) +
                  '</div>';
                }).join('') + '</div>';
                const arrow = i < levels.length - 1 ? '<div class="dag-arrow">&#x25BC;</div>' : '';
                return nodes + arrow;
              }).join('')}
            </div>
          </div>
        </div>` : ''}

        ${resources.length ? `<div class="detail-section">
          <h3>Resource Status</h3>
          <div class="table-container"><table>
            <thead><tr><th>Resource</th><th>Environment</th><th>Status</th><th>Outputs</th></tr></thead>
            <tbody>${resources.map(r => `
              <tr>
                <td><span class="resource-name">${esc(r.name)}</span></td>
                <td>${r.environment ? badge(r.environment, 'info') : '-'}</td>
                <td>${phaseBadge(r.phase)}</td>
                <td>${r.outputs ? '<code style="font-size:12px">' + esc(JSON.stringify(r.outputs).substring(0, 120)) + '</code>' : r.error ? '<span style="color:var(--danger);font-size:12px">' + esc(r.error) + '</span>' : '-'}</td>
              </tr>
            `).join('')}</tbody>
          </table></div>
        </div>` : ''}

        ${decisions.length ? `<div class="detail-section">
          <h3>Placement Decisions</h3>
          ${decisions.map(dec => `
            <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-md);padding:14px;margin-bottom:10px">
              <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
                <strong>${esc(dec.Resource)}</strong>
                ${dec.Selected ? badge(dec.Selected, 'success') : badge('FAILED', 'danger')}
              </div>
              ${dec.Candidates?.length ? '<table style="font-size:13px"><thead><tr><th>Env</th><th>Eligible</th><th>Score</th></tr></thead><tbody>' +
                dec.Candidates.map(c => '<tr><td>' + esc(c.Environment) + '</td><td>' + (c.Eligible ? badge('yes','success') : badge('no','danger')) + '</td><td>' + (c.Eligible ? c.Score.toFixed(2) : '-') + '</td></tr>').join('') +
                '</tbody></table>' : ''}
            </div>
          `).join('')}
        </div>` : ''}
      </div>
    `;
  }

  function buildSimpleDAG(resources) {
    if (!resources.length) return [];
    const deps = {};
    const names = new Set(resources.map(r => r.name));
    resources.forEach(r => {
      deps[r.name] = new Set();
      (r.requirements || []).forEach(d => { if (names.has(d)) deps[r.name].add(d); });
      // Scan for ${ref.field} patterns in properties
      const scan = (val) => {
        if (typeof val === 'string') {
          const matches = val.matchAll(/\$\{(\w+)\.\w+\}/g);
          for (const m of matches) { if (names.has(m[1]) && m[1] !== r.name) deps[r.name].add(m[1]); }
        } else if (val && typeof val === 'object') {
          Object.values(val).forEach(scan);
        }
      };
      if (r.properties) scan(r.properties);
    });

    const levels = [];
    const placed = new Set();
    while (placed.size < names.size) {
      const level = [];
      for (const name of names) {
        if (placed.has(name)) continue;
        const unmet = [...deps[name]].filter(d => !placed.has(d));
        if (unmet.length === 0) level.push(name);
      }
      if (level.length === 0) break;
      level.sort();
      levels.push(level);
      level.forEach(n => placed.add(n));
    }
    return levels;
  }

  function renderDAG(levels, resources) {
    const typeMap = {};
    resources.forEach(r => { typeMap[r.name] = r.type; });

    return '<div class="dag-levels">' +
      levels.map((level, i) => {
        const nodes = '<div class="dag-level">' +
          level.map(name =>
            `<div class="dag-node">
              <div class="dag-node-name">${esc(name)}</div>
              <div class="dag-node-type">${esc(typeMap[name] || '')}</div>
            </div>`
          ).join('') + '</div>';
        const arrow = i < levels.length - 1 ? '<div class="dag-arrow">&#x25BC;</div>' : '';
        return nodes + arrow;
      }).join('') + '</div>';
  }

  // --- Environments ---

  async function renderEnvironments(el) {
    const data = await api('environments');
    const items = data.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Environments</h2>
          <p>${items.length} environment${items.length !== 1 ? 's' : ''} registered</p>
        </div>
        <button class="create-btn" id="create-env-btn">+ Create Environment</button>
      </div>
      ${items.length ? '<div class="cards-grid">' + items.map(env => `
        <div class="card" onclick="location.hash='#/environments/${env.metadata.name}'">
          <div class="card-header">
            <div>
              <div class="card-title">${esc(env.metadata.name)}</div>
              <div class="card-subtitle">${esc(env.spec.description || '')}</div>
            </div>
            ${typeBadge(env.spec.type)}
          </div>
          ${kvGrid([
            ['Country', esc(env.spec.sovereignty?.country || '-')],
            ['Jurisdiction', esc(env.spec.sovereignty?.jurisdiction || '-')],
            ['Resource Types', String(env.spec.capabilities?.resourceTypes?.length || 0)],
          ])}
          ${env.metadata.labels ? '<div style="margin-top:12px">' + tags(env.metadata.labels) + '</div>' : ''}
        </div>
      `).join('') + '</div>' :
        '<div class="empty-state"><div class="empty-state-icon">&#9729;</div><h3>No environments</h3><p>Click "Create Environment" to register one.</p></div>'}
    `;

    $('#create-env-btn', el).onclick = () =>
      showCreateModal('Create Environment', TMPL_ENV, 'environments', () => navigate());
  }

  async function renderEnvironmentDetail(el, params) {
    const env = await apiGet('environments', params.name);
    if (!env) {
      el.innerHTML = `${backLink('#/environments', 'Environments')}<div class="empty-state"><h3>Environment not found</h3></div>`;
      return;
    }

    const s = env.spec;
    const sov = s.sovereignty || {};

    el.innerHTML = `
      ${backLink('#/environments', 'Environments')}
      <div class="detail-header">
        <div>
          <div class="detail-title">${esc(env.metadata.name)}</div>
          <div class="detail-meta">${esc(s.description || s.type)}</div>
        </div>
        <div style="display:flex;gap:10px;align-items:center">
          ${typeBadge(s.type)}
          <button class="edit-btn" id="edit-env-btn">Edit</button>
          <button class="delete-btn" id="delete-env-btn">Delete</button>
        </div>
      </div>

      ${env.metadata.labels ? `<div class="detail-section"><h3>Labels</h3>${tags(env.metadata.labels)}</div>` : ''}

      <div class="detail-section">
        <h3>Connection</h3>
        ${kvGrid([
          ['Endpoint', esc(s.connection?.endpoint || '-')],
          ['Credentials', esc(s.connection?.credentialRef || '-')],
        ])}
      </div>

      <div class="detail-section">
        <h3>Sovereignty</h3>
        ${kvGrid([
          ['Country', esc(sov.country || '-')],
          ['Region', esc(sov.region || '-')],
          ['Zone', esc(sov.zone || '-')],
          ['Jurisdiction', esc(sov.jurisdiction || '-')],
          ['Data Classification', sov.dataClassification ? phaseBadge(sov.dataClassification) : '-'],
          ['Compliance', sov.compliance?.length ? sov.compliance.map(c => badge(c, 'info')).join(' ') : '-'],
        ])}
      </div>

      <div class="detail-section">
        <h3>Capabilities</h3>
        <div style="margin-bottom:12px"><strong style="color:var(--text-secondary);font-size:12px">RESOURCE TYPES</strong></div>
        <div class="tags">
          ${(s.capabilities?.resourceTypes || []).map(rt => `<span class="tag">${esc(rt)}</span>`).join('')}
        </div>
        ${s.capabilities?.features?.length ? `
          <div style="margin:16px 0 12px"><strong style="color:var(--text-secondary);font-size:12px">FEATURES</strong></div>
          <div class="tags">${s.capabilities.features.map(f => `<span class="tag">${esc(f)}</span>`).join('')}</div>
        ` : ''}
      </div>

      ${s.capacity ? `<div class="detail-section"><h3>Capacity</h3>${kvGrid(
        Object.entries(s.capacity).filter(([k]) => k !== 'custom').map(([k, v]) =>
          [k.toUpperCase(), `${v.total} ${v.unit}`]
        )
      )}</div>` : ''}

      ${s.cost ? `<div class="detail-section"><h3>Cost Rates</h3>
        <div class="table-container"><table>
          <thead><tr><th>Resource</th><th>Rate</th><th>Unit</th></tr></thead>
          <tbody>${Object.entries(s.cost.rates || {}).filter(([,v]) => v).map(([k, v]) =>
            `<tr><td>${esc(k)}</td><td>${v.value} ${esc(s.cost.currency)}</td><td>${esc(v.unit)}</td></tr>`
          ).join('')}</tbody>
        </table></div>
      </div>` : ''}

      ${s.networking?.overlays?.length ? `<div class="detail-section"><h3>Overlay Networks</h3>
        <div class="table-container"><table>
          <thead><tr><th>Name</th><th>Type</th><th>Latency</th><th>Bandwidth</th></tr></thead>
          <tbody>${s.networking.overlays.map(o =>
            `<tr><td>${esc(o.name)}</td><td>${typeBadge(o.type)}</td><td>${esc(o.latency || '-')}</td><td>${esc(o.bandwidth || '-')}</td></tr>`
          ).join('')}</tbody>
        </table></div>
      </div>` : ''}
    `;

    $('#edit-env-btn', el).onclick = () =>
      showEditModal('Edit Environment', 'environments', env.metadata.name, env._revision, () => navigate());
    $('#delete-env-btn', el).onclick = () =>
      showDeleteConfirm('Environment', env.metadata.name, 'environments', env._revision, '#/environments');
  }

  // --- Resource Types ---

  async function renderResourceTypes(el) {
    const data = await api('resourcetypes');
    const items = data.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Resource Types</h2>
          <p>${items.length} resource type${items.length !== 1 ? 's' : ''} in catalog</p>
        </div>
        <button class="create-btn" id="create-rt-btn">+ Create Resource Type</button>
      </div>
      ${items.length ? '<div class="cards-grid">' + items.map(rt => {
        const schema = rt.spec?.schema;
        const props = schema?.properties ? Object.keys(schema.properties) : [];
        const inputs = props.filter(p => !schema.properties[p].readOnly);
        const outputs = props.filter(p => schema.properties[p].readOnly);
        return `
          <div class="card" onclick="location.hash='#/resourcetypes/${rt.metadata.name}'">
            <div class="card-header">
              <div>
                <div class="card-title">${esc(rt.metadata.name)}</div>
                <div class="card-subtitle">v${esc(rt.spec.version)}</div>
              </div>
              ${phaseBadge(rt.spec.lifecycle)}
            </div>
            ${kvGrid([
              ['Inputs', String(inputs.length)],
              ['Outputs', String(outputs.length)],
            ])}
            ${rt.metadata.labels ? '<div style="margin-top:12px">' + tags(rt.metadata.labels) + '</div>' : ''}
          </div>`;
      }).join('') + '</div>' :
        '<div class="empty-state"><div class="empty-state-icon">&#128204;</div><h3>No resource types</h3><p>Click "Create Resource Type" to define one.</p></div>'}
    `;

    $('#create-rt-btn', el).onclick = () =>
      showCreateModal('Create Resource Type', TMPL_RT, 'resourcetypes', () => navigate());
  }

  async function renderResourceTypeDetail(el, params) {
    const rt = await apiGet('resourcetypes', params.name);
    if (!rt) {
      el.innerHTML = `${backLink('#/resourcetypes', 'Resource Types')}<div class="empty-state"><h3>Resource type not found</h3></div>`;
      return;
    }

    const schema = rt.spec?.schema || {};
    const props = schema.properties || {};
    const required = new Set(schema.required || []);
    const inputs = Object.entries(props).filter(([, v]) => !v.readOnly);
    const outputs = Object.entries(props).filter(([, v]) => v.readOnly);

    function propRow([name, p]) {
      const flags = [];
      if (required.has(name)) flags.push(badge('required', 'warning'));
      if (p.readOnly) flags.push(badge('readOnly', 'info'));
      if (p['x-dcm-sensitive']) flags.push(badge('sensitive', 'danger'));
      return `<tr>
        <td><span class="resource-name">${esc(name)}</span></td>
        <td>${esc(p.type || '-')}</td>
        <td>${esc(p.description || '-')}</td>
        <td>${p.enum ? esc(p.enum.join(', ')) : p.minimum != null ? `${p.minimum} - ${p.maximum}` : p.default != null ? `default: ${p.default}` : '-'}</td>
        <td>${flags.join(' ') || '-'}</td>
      </tr>`;
    }

    el.innerHTML = `
      ${backLink('#/resourcetypes', 'Resource Types')}
      <div class="detail-header">
        <div>
          <div class="detail-title">${esc(rt.metadata.name)}</div>
          <div class="detail-meta">v${esc(rt.spec.version)}</div>
        </div>
        <div style="display:flex;align-items:center;gap:12px">
          ${phaseBadge(rt.spec.lifecycle)}
          <button class="delete-btn" id="delete-rt-btn">Delete</button>
        </div>
      </div>

      ${rt.metadata.labels ? `<div class="detail-section"><h3>Labels</h3>${tags(rt.metadata.labels)}</div>` : ''}

      ${rt.spec.deprecation ? `<div class="detail-section" style="background:var(--danger-bg);padding:16px;border-radius:var(--radius-md)">
        <strong style="color:var(--danger)">Deprecated:</strong> ${esc(rt.spec.deprecation.message)} (deadline: ${esc(rt.spec.deprecation.deadline)})
      </div>` : ''}

      <div class="detail-section">
        <h3>Input Properties (${inputs.length})</h3>
        ${inputs.length ? `<div class="table-container"><table>
          <thead><tr><th>Name</th><th>Type</th><th>Description</th><th>Constraints</th><th>Flags</th></tr></thead>
          <tbody>${inputs.map(propRow).join('')}</tbody>
        </table></div>` : '<p style="color:var(--text-muted)">No input properties</p>'}
      </div>

      <div class="detail-section">
        <h3>Output Properties (${outputs.length})</h3>
        ${outputs.length ? `<div class="table-container"><table>
          <thead><tr><th>Name</th><th>Type</th><th>Description</th><th>Constraints</th><th>Flags</th></tr></thead>
          <tbody>${outputs.map(propRow).join('')}</tbody>
        </table></div>` : '<p style="color:var(--text-muted)">No output properties</p>'}
      </div>
    `;

    $('#delete-rt-btn', el).onclick = () =>
      showDeleteConfirm('Resource Type', rt.metadata.name, 'resourcetypes', rt._revision, '#/resourcetypes');
  }

  // --- Recipes ---

  async function renderRecipes(el) {
    const data = await api('recipes');
    const items = data.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Recipes</h2>
          <p>${items.length} recipe${items.length !== 1 ? 's' : ''} registered</p>
        </div>
        <button class="create-btn" id="create-recipe-btn">+ Create Recipe</button>
      </div>
      ${items.length ? `<div class="table-container"><table>
        <thead><tr><th>Name</th><th>Resource Type</th><th>Type</th><th>Source</th><th>Env Match</th></tr></thead>
        <tbody>${items.map(r => `
          <tr>
            <td><strong>${esc(r.metadata.name)}</strong></td>
            <td>${badge(r.spec.resourceType, 'accent')}</td>
            <td>${typeBadge(r.spec.type)}</td>
            <td><code style="font-size:12px">${esc(Object.entries(r.spec.source || {}).map(([k,v]) => `${k}: ${v}`).join(', '))}</code></td>
            <td>${r.spec.environmentMatch?.types?.map(t => typeBadge(t)).join(' ') || badge('all', 'neutral')}</td>
          </tr>
        `).join('')}</tbody>
      </table></div>` :
        '<div class="empty-state"><div class="empty-state-icon">&#128196;</div><h3>No recipes</h3><p>Click "Create Recipe" to register one.</p></div>'}
    `;

    $('#create-recipe-btn', el).onclick = () =>
      showCreateModal('Create Recipe', TMPL_RECIPE, 'recipes', () => navigate());
  }

  // --- Deployments ---

  async function renderDeployments(el) {
    const data = await api('deployments');
    const items = (data.items || []).sort((a, b) =>
      (b.spec?.startedAt || '').localeCompare(a.spec?.startedAt || '')
    );

    el.innerHTML = `
      <div class="page-header">
        <h2>Deployments</h2>
        <p>${items.length} deployment${items.length !== 1 ? 's' : ''} recorded</p>
      </div>
      ${items.length ? `<div class="table-container"><table>
        <thead><tr><th>Name</th><th>Application</th><th>Status</th><th>Resources</th><th>Started</th><th>Duration</th></tr></thead>
        <tbody>${items.map(dep => {
          const s = dep.spec || {};
          const started = s.startedAt ? new Date(s.startedAt) : null;
          const finished = s.finishedAt ? new Date(s.finishedAt) : null;
          const duration = started && finished ? formatDuration(finished - started) : '-';
          const timeAgo = started ? formatTimeAgo(started) : '-';
          return `<tr style="cursor:pointer" onclick="location.hash='#/deployments/${dep.metadata.name}'">
            <td><strong>${esc(dep.metadata.name)}</strong></td>
            <td><a href="#/applications/${s.application}" style="color:var(--accent);text-decoration:none">${esc(s.application)}</a></td>
            <td>${phaseBadge(s.phase)}</td>
            <td>${(s.resources || []).length}</td>
            <td title="${started ? started.toISOString() : ''}">${esc(timeAgo)}</td>
            <td>${esc(duration)}</td>
          </tr>`;
        }).join('')}</tbody>
      </table></div>` :
        '<div class="empty-state"><div class="empty-state-icon">&#128640;</div><h3>No deployments yet</h3><p>Deploy an Application to see deployment history here.</p></div>'}
    `;
  }

  async function renderDeploymentDetail(el, params) {
    const dep = await apiGet('deployments', params.name);
    if (!dep) {
      el.innerHTML = `${backLink('#/deployments', 'Deployments')}<div class="empty-state"><h3>Deployment not found</h3></div>`;
      return;
    }

    const s = dep.spec || {};
    const started = s.startedAt ? new Date(s.startedAt) : null;
    const finished = s.finishedAt ? new Date(s.finishedAt) : null;
    const duration = started && finished ? formatDuration(finished - started) : '-';
    const isSuccess = s.phase === 'Provisioned';
    const levels = s.levels || [];
    const resources = s.resources || [];
    const assignments = s.assignments || {};

    el.innerHTML = `
      ${backLink('#/deployments', 'Deployments')}
      <div class="detail-header">
        <div>
          <div class="detail-title">${esc(dep.metadata.name)}</div>
          <div class="detail-meta">
            Application: <a href="#/applications/${s.application}" style="color:var(--accent);text-decoration:none">${esc(s.application)}</a>
          </div>
        </div>
        ${phaseBadge(s.phase)}
      </div>

      <div class="detail-section">
        <h3>Summary</h3>
        ${kvGrid([
          ['Status', phaseBadge(s.phase)],
          ['Started', started ? started.toLocaleString() : '-'],
          ['Finished', finished ? finished.toLocaleString() : '-'],
          ['Duration', esc(duration)],
          ['Resources', String(resources.length)],
        ])}
        ${s.error ? `<div style="margin-top:14px;background:var(--danger-bg);padding:12px;border-radius:var(--radius-md);border:1px solid rgba(239,68,68,0.3)">
          <strong style="color:var(--danger)">Error:</strong>
          <pre style="margin-top:6px;color:var(--text-secondary);font-size:13px;white-space:pre-wrap">${esc(s.error)}</pre>
        </div>` : ''}
      </div>

      ${levels.length ? `<div class="detail-section">
        <h3>Execution Order</h3>
        <div class="dag-container">
          <div class="dag-levels">
            ${levels.map((level, i) => {
              const nodes = '<div class="dag-level">' + level.map(name => {
                const rs = resources.find(r => r.name === name);
                const phase = rs?.phase || 'Pending';
                const nodeColor = phase === 'Provisioned' ? 'var(--success)' : phase === 'Failed' ? 'var(--danger)' : 'var(--border)';
                return '<div class="dag-node" style="border-color:' + nodeColor + '">' +
                  '<div class="dag-node-name">' + esc(name) + '</div>' +
                  '<div class="dag-node-type">' + esc(assignments[name] || '') + '</div>' +
                  phaseBadge(phase) +
                '</div>';
              }).join('') + '</div>';
              const arrow = i < levels.length - 1 ? '<div class="dag-arrow">&#x25BC;</div>' : '';
              return nodes + arrow;
            }).join('')}
          </div>
        </div>
      </div>` : ''}

      ${resources.length ? `<div class="detail-section">
        <h3>Resources</h3>
        <div class="table-container"><table>
          <thead><tr><th>Name</th><th>Environment</th><th>Status</th><th>Outputs</th></tr></thead>
          <tbody>${resources.map(r => `
            <tr>
              <td><span class="resource-name">${esc(r.name)}</span></td>
              <td>${r.environment ? badge(r.environment, 'info') : '-'}</td>
              <td>${phaseBadge(r.phase)}</td>
              <td>${r.outputs && Object.keys(r.outputs).length ? '<code style="font-size:12px">' + esc(JSON.stringify(r.outputs)) + '</code>' : r.error ? '<span style="color:var(--danger);font-size:12px">' + esc(r.error) + '</span>' : '-'}</td>
            </tr>
          `).join('')}</tbody>
        </table></div>
      </div>` : ''}
    `;
  }

  function formatDuration(ms) {
    if (ms < 1000) return ms + 'ms';
    const secs = Math.floor(ms / 1000);
    if (secs < 60) return secs + 's';
    const mins = Math.floor(secs / 60);
    return mins + 'm ' + (secs % 60) + 's';
  }

  function formatTimeAgo(date) {
    const diff = Date.now() - date.getTime();
    const secs = Math.floor(diff / 1000);
    if (secs < 60) return 'just now';
    const mins = Math.floor(secs / 60);
    if (mins < 60) return mins + 'm ago';
    const hours = Math.floor(mins / 60);
    if (hours < 24) return hours + 'h ago';
    const days = Math.floor(hours / 24);
    return days + 'd ago';
  }

  // --- Policies ---

  // --- Placement ---

  async function renderPlacement(el) {
    const [apps, envs, policies] = await Promise.all([
      api('applications'), api('environments'), api('placementpolicies'),
    ]);
    const appItems = apps.items || [];
    const envItems = envs.items || [];
    const policyItems = policies.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Placement Simulator</h2>
          <p>Simulate where Application resources would be placed based on current environments and policies.</p>
        </div>
        <button class="create-btn" id="create-placement-policy-btn">+ Create Policy</button>
      </div>

      <div class="detail-section">
        <h3>Select Application</h3>
        ${appItems.length ? `
          <div class="cards-grid">
            ${appItems.map(app => `
              <div class="card placement-app-card" data-app="${esc(app.metadata.name)}" style="cursor:pointer">
                <div class="card-header">
                  <div>
                    <div class="card-title">${esc(app.metadata.name)}</div>
                    <div class="card-subtitle">${(app.spec?.resources || []).length} resources</div>
                  </div>
                  <button class="simulate-btn" data-app="${esc(app.metadata.name)}">Simulate</button>
                </div>
              </div>
            `).join('')}
          </div>
        ` : '<div class="empty-state"><h3>No applications</h3><p>Create an Application first.</p></div>'}
      </div>

      <div class="detail-section">
        <h3>Current Environments (${envItems.length})</h3>
        <div class="table-container"><table>
          <thead><tr><th>Name</th><th>Type</th><th>Resource Types</th><th>Jurisdiction</th><th>Features</th></tr></thead>
          <tbody>${envItems.map(env => `
            <tr>
              <td><strong>${esc(env.metadata.name)}</strong></td>
              <td>${typeBadge(env.spec.type)}</td>
              <td>${(env.spec.capabilities?.resourceTypes || []).map(rt => badge(rt, 'accent')).join(' ')}</td>
              <td>${esc(env.spec.sovereignty?.jurisdiction || '-')} (${esc(env.spec.sovereignty?.country || '')})</td>
              <td>${(env.spec.capabilities?.features || []).map(f => badge(f, 'neutral')).join(' ') || '-'}</td>
            </tr>
          `).join('')}</tbody>
        </table></div>
      </div>

      <div class="detail-section">
        <h3>Active Policies (${policyItems.length})</h3>
        ${policyItems.length ? `<div class="table-container"><table>
          <thead><tr><th>Name</th><th>Match</th><th>Rule</th><th>Prefer</th><th>Weight</th><th>Priority</th></tr></thead>
          <tbody>${policyItems.map(p => {
            const m = p.spec.match || {};
            let matchDesc = '-';
            if (m.all) matchDesc = 'all';
            else if (m.labels) matchDesc = Object.entries(m.labels).map(([k,v]) => k+'='+v).join(', ');
            else if (m.resourceTypes) matchDesc = m.resourceTypes.join(', ');
            return `<tr>
              <td><strong>${esc(p.metadata.name)}</strong></td>
              <td>${esc(matchDesc)}</td>
              <td><code style="font-size:12px;color:var(--warning)">${esc(p.spec.rule || '-')}</code></td>
              <td><code style="font-size:12px;color:var(--success)">${esc(p.spec.prefer || '-')}</code></td>
              <td>${p.spec.weight || 1.0}</td>
              <td>${p.spec.priority || 0}</td>
            </tr>`;
          }).join('')}</tbody>
        </table></div>` : '<p style="color:var(--text-muted)">No policies defined. Resources will be placed in any eligible environment.</p>'}
      </div>

      <div id="placement-result"></div>
    `;

    // Attach click handlers
    el.querySelectorAll('.simulate-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const appName = btn.dataset.app;
        await runPlacementSimulation(appName);
      });
    });

    $('#create-placement-policy-btn', el).onclick = () =>
      showCreateModal('Create Placement Policy', TMPL_POLICY, 'placementpolicies', () => navigate());
  }

  async function runPlacementSimulation(appName) {
    const resultEl = document.getElementById('placement-result');
    resultEl.innerHTML = '<div class="loading"><div class="spinner"></div>Running placement simulation...</div>';

    const resp = await fetch(`${API}/placement/${encodeURIComponent(appName)}`);
    const data = await resp.json();

    if (data.error && !data.assignments) {
      resultEl.innerHTML = `
        <div class="detail-section">
          <h3>Placement Result: ${esc(appName)}</h3>
          <div style="background:var(--danger-bg);padding:16px;border-radius:var(--radius-md);border:1px solid rgba(239,68,68,0.3)">
            <strong style="color:var(--danger)">Placement Failed</strong>
            <pre style="margin-top:8px;font-size:13px;color:var(--text-secondary);white-space:pre-wrap">${esc(data.error)}</pre>
          </div>
        </div>`;
      return;
    }

    const assignments = data.assignments || {};
    const decisions = data.decisions || [];

    resultEl.innerHTML = `
      <div class="detail-section">
        <h3>Placement Result: ${esc(appName)}</h3>
        ${data.error ? `<div style="background:var(--warning-bg);padding:12px;border-radius:var(--radius-md);margin-bottom:16px;border:1px solid rgba(245,158,11,0.3)">
          <strong style="color:var(--warning)">Warning:</strong> ${esc(data.error)}
        </div>` : `<div style="background:var(--success-bg);padding:12px;border-radius:var(--radius-md);margin-bottom:16px;border:1px solid rgba(34,197,94,0.3)">
          <strong style="color:var(--success)">Placement Succeeded</strong> — ${Object.keys(assignments).length} resource(s) placed
        </div>`}

        <div class="detail-section">
          <h3>Assignments</h3>
          <div class="table-container"><table>
            <thead><tr><th>Resource</th><th>Environment</th></tr></thead>
            <tbody>${Object.entries(assignments).map(([res, env]) => `
              <tr>
                <td><span class="resource-name">${esc(res)}</span></td>
                <td>${badge(env, 'success')}</td>
              </tr>
            `).join('')}</tbody>
          </table></div>
        </div>

        <div class="detail-section">
          <h3>Decision Log</h3>
          ${decisions.map(dec => `
            <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-md);padding:16px;margin-bottom:12px">
              <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
                <strong style="font-size:15px">${esc(dec.Resource)}</strong>
                ${dec.Selected ? badge(dec.Selected, 'success') : badge('FAILED', 'danger')}
              </div>
              ${dec.FailedReason ? `<div style="color:var(--danger);font-size:13px;margin-bottom:8px">${esc(dec.FailedReason)}</div>` : ''}
              ${dec.Candidates?.length ? `
                <table style="font-size:13px">
                  <thead><tr><th>Environment</th><th>Eligible</th><th>Score</th><th>Details</th></tr></thead>
                  <tbody>${dec.Candidates.map(c => `
                    <tr>
                      <td>${esc(c.Environment)}</td>
                      <td>${c.Eligible ? badge('yes', 'success') : badge('no', 'danger')}</td>
                      <td>${c.Eligible ? c.Score.toFixed(2) : '-'}</td>
                      <td>${c.Eliminations?.length ? c.Eliminations.map(e => `<div style="color:var(--danger);font-size:12px">${esc(e)}</div>`).join('') : c.Eligible ? '<span style="color:var(--success)">passed all rules</span>' : '-'}</td>
                    </tr>
                  `).join('')}</tbody>
                </table>
              ` : ''}
            </div>
          `).join('')}
        </div>
      </div>
    `;
  }

  // --- Policies ---

  async function renderPolicies(el) {
    const data = await api('placementpolicies');
    const items = data.items || [];

    el.innerHTML = `
      <div class="page-header page-header-row">
        <div>
          <h2>Placement Policies</h2>
          <p>${items.length} polic${items.length !== 1 ? 'ies' : 'y'} defined</p>
        </div>
        <button class="create-btn" id="create-policy-btn">+ Create Policy</button>
      </div>
      ${items.length ? `<div class="cards-grid">${items.map(p => {
        const m = p.spec.match || {};
        let matchDesc = 'all';
        if (m.all) matchDesc = 'All applications';
        else if (m.labels) matchDesc = Object.entries(m.labels).map(([k,v]) => `${k}=${v}`).join(', ');
        else if (m.resourceTypes) matchDesc = m.resourceTypes.join(', ');

        return `<div class="card">
          <div class="card-header">
            <div>
              <div class="card-title">${esc(p.metadata.name)}</div>
              <div class="card-subtitle">Priority: ${p.spec.priority || 0} | Weight: ${p.spec.weight || 1.0}</div>
            </div>
          </div>
          <div class="kv-grid">
            <div class="kv-item">
              <div class="kv-label">Match</div>
              <div class="kv-value" style="font-size:13px">${esc(matchDesc)}</div>
            </div>
            ${p.spec.rule ? `<div class="kv-item">
              <div class="kv-label">Rule</div>
              <div class="kv-value" style="font-size:12px;color:var(--warning)">${esc(p.spec.rule)}</div>
            </div>` : ''}
            ${p.spec.prefer ? `<div class="kv-item">
              <div class="kv-label">Prefer</div>
              <div class="kv-value" style="font-size:12px;color:var(--success)">${esc(p.spec.prefer)}</div>
            </div>` : ''}
          </div>
          ${p.metadata.labels ? '<div style="margin-top:12px">' + tags(p.metadata.labels) + '</div>' : ''}
        </div>`;
      }).join('')}</div>` :
        '<div class="empty-state"><div class="empty-state-icon">&#128737;</div><h3>No policies</h3><p>Click "Create Policy" to define one.</p></div>'}
    `;

    $('#create-policy-btn', el).onclick = () =>
      showCreateModal('Create Placement Policy', TMPL_POLICY, 'placementpolicies', () => navigate());
  }

})();

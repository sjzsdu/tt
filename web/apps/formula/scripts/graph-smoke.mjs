import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { rm } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { readFile, writeFile } from 'node:fs/promises';

const root = process.cwd();
const packageEntry = join(root, 'src/components/graph/graphModel.ts');
const repoEntry = join(root, 'web/apps/formula/src/components/graph/graphModel.ts');
const entry = existsSync(packageEntry) ? packageEntry : repoEntry;
const outfile = join(tmpdir(), `formula-graph-model-smoke-${process.pid}.mjs`);
const executionEntry = join(dirname(entry), '../../utils/execution.ts');
const executionOutfile = join(tmpdir(), `formula-execution-smoke-${process.pid}.mjs`);

if (!existsSync(entry)) {
  throw new Error(`Missing graph model entry: ${entry}`);
}

const source = await readFile(entry, 'utf8');
let transpiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
  },
}).outputText;
const executionSource = await readFile(executionEntry, 'utf8');
const executionTranspiled = ts.transpileModule(executionSource, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
  },
}).outputText;
transpiled = transpiled.replace('../../utils/execution', `./formula-execution-smoke-${process.pid}.mjs`);
await writeFile(outfile, transpiled);
await writeFile(executionOutfile, executionTranspiled);

try {
  const {
    computeGraphData,
    computeLiveGraphData,
    computeLoopBodyGraphData,
    loopBodyGraphID,
    loopBodyStep,
    resolveClickedStep,
  } = await import(pathToFileURL(outfile));
  const { parseExecutionAddress } = await import(pathToFileURL(executionOutfile));

  const loopStep = {
    id: 'collect',
    title: 'Collect data',
    agent: 'analyst',
    status: 'running',
    index: 1,
    depends_on: ['prepare'],
    activities: [
      { at: '2026-06-08T00:00:00Z', step_id: 'collect.iter2.fetch', status: 'completed', output: 'ok', duration_ms: 12 },
    ],
    loop: {
      summary: 'collect items',
      body: [
        { id: 'fetch', title: 'Fetch item', output_key: 'item' },
        { id: 'summarize', title: 'Summarize item', depends_on: ['fetch'] },
      ],
    },
  };

  const snapshot = {
    recipe_name: 'smoke',
    status: 'running',
    logs: [],
    edges: [],
    steps: [
      { id: 'prepare', title: 'Prepare', agent: 'analyst', status: 'completed', index: 0 },
      loopStep,
      { id: 'publish', title: 'Publish', agent: 'writer', status: 'pending', index: 2, depends_on: ['collect'] },
      { id: 'audit', title: 'Audit', agent: 'reviewer', status: 'pending', index: 3, depends_on: ['collect'] },
    ],
  };

  const collapsed = computeGraphData(snapshot, new Set());
  assert.equal(collapsed.nodes.length, 4, 'collapsed graph should only contain top-level steps');
  assert.equal(collapsed.combos.length, 0, 'collapsed graph should not contain loop combo');
  const prepareToCollect = collapsed.edges.find(edge => edge.id === 'prepare-collect');
  assert.equal(prepareToCollect?.data.sourcePort, 'bottom', 'single-source edge should leave from the middle bottom port');
  assert.equal(prepareToCollect?.data.targetPort, 'top', 'single-target edge should enter from the middle top port');
  assert.ok(collapsed.edges.some(edge => edge.id === 'prepare-collect'), 'depends_on should synthesize prepare -> collect edge');
  assert.ok(collapsed.edges.some(edge => edge.id === 'collect-publish'), 'depends_on should synthesize collect -> publish edge');
  assert.ok(collapsed.edges.some(edge => edge.id === 'collect-audit'), 'depends_on should synthesize collect -> audit edge');
  const collectOverview = collapsed.nodes.find(node => node.id === 'collect');
  assert.match(collectOverview?.data.step.metadata?.execution_summary || '', /running|completed/, 'overview loop node should summarize concrete instance states');
  const collectOutgoing = collapsed.edges.filter(edge => edge.source === 'collect').sort((a, b) => a.target.localeCompare(b.target));
  assert.equal(new Set(collectOutgoing.map(edge => edge.data.sourcePort)).size, collectOutgoing.length, 'same-source edges should use distinct source ports');
  assert.deepEqual(collectOutgoing.map(edge => edge.data.sourcePort).sort(), ['bottom-left-1', 'bottom-right-1'], 'two same-source edges should use balanced ports around the center');
  assert.ok(collectOutgoing.some(edge => edge.data.laneOffset !== 0), 'same-source edges should receive non-zero lane offsets');

  const expanded = computeGraphData(snapshot, new Set(['collect']));
  const fetchNodeID = loopBodyGraphID('collect', 'fetch');
  const summarizeNodeID = loopBodyGraphID('collect', 'summarize');
  const fetchBodyStep = loopBodyStep(loopStep, loopStep.loop.body[0], 0, loopStep);

  assert.equal(expanded.nodes.length, 6, 'expanded graph should add loop body nodes as an independent graph island');
  assert.equal(expanded.combos.length, 0, 'expanded graph should not create loop combos in the main graph');
  const fetchNode = expanded.nodes.find(node => node.id === fetchNodeID);
  const summarizeNode = expanded.nodes.find(node => node.id === summarizeNodeID);
  const collectNode = expanded.nodes.find(node => node.id === 'collect');
  assert.equal(fetchNode?.data.kind, 'loop-body', 'expanded loop body should render as a graph node');
  assert.equal(summarizeNode?.data.kind, 'loop-body', 'expanded loop dependency target should render as a graph node');
  assert.ok(expanded.edges.some(edge => edge.source === fetchNodeID && edge.target === summarizeNodeID), 'loop body dependencies should render as graph edges');
  const collectEntryEdge = expanded.edges.find(edge => edge.id === `collect-loop-entry-${fetchNodeID}`);
  assert.equal(collectEntryEdge?.data.kind, 'loop-expand', 'expanded loop body island should have a visual parent connector edge');
  assert.equal(collectEntryEdge?.data.sourcePort, 'right', 'parent connector should leave from the parent loop right side');
  assert.equal(collectEntryEdge?.data.targetPort, 'left', 'parent connector should enter the child island from the left side');
  assert.equal(expanded.nodes.find(node => node.id === 'collect')?.data.expanded, true, 'loop node should still expose expanded state for the +/- badge');
  for (const collapsedNode of collapsed.nodes) {
    const expandedNode = expanded.nodes.find(node => node.id === collapsedNode.id);
    assert.deepEqual(
      [expandedNode?.style.x, expandedNode?.style.y],
      [collapsedNode.style.x, collapsedNode.style.y],
      `expanding a loop should not change top-level coordinates for ${collapsedNode.id}`,
    );
  }
  assert.ok(fetchNode.style.x > collectNode.style.x, 'loop body graph island should be positioned to the right of its parent loop step');
  assert.ok(summarizeNode.style.y > fetchNode.style.y, 'loop body graph should use its own dependency-depth layout');
  assert.equal(fetchBodyStep.id, fetchNodeID, 'loop body helper should still materialize the graph-island body id');
  assert.equal(fetchBodyStep.status, 'completed', 'loop body helper should inherit latest matching activity status');
  assert.equal(fetchBodyStep.metadata.iteration, '2', 'loop body helper should expose latest iteration metadata');
  const independentBodyGraph = computeLoopBodyGraphData(loopStep, 'collect', loopStep, new Set(['collect']));
  assert.deepEqual(
    independentBodyGraph.nodes.map(node => node.id).sort(),
    [fetchNodeID, summarizeNodeID].sort(),
    'loop body graph helper should model only the loop body island',
  );

  const live = computeLiveGraphData(snapshot);
  assert.ok(live.nodes.some(node => node.id === 'collect.iter2.fetch'), 'live graph should materialize concrete loop addresses');
  assert.equal(live.nodes.find(node => node.id === 'collect.iter2.fetch')?.data.step.status, 'completed', 'live node should keep concrete instance status');
  assert.deepEqual(
    parseExecutionAddress('outer.iter3.inner.iter2.summarize').iterationPath,
    [3, 2],
    'execution address parser should support nested loops',
  );

  const nestedLoopStep = {
    id: 'outer',
    title: 'Outer loop',
    agent: 'analyst',
    status: 'running',
    index: 0,
    loop: {
      body: [
        {
          id: 'inner-loop',
          title: 'Inner loop',
          loop: {
            body: [
              { id: 'inner-fetch', title: 'Inner fetch', var_refs: ['repo'] },
              { id: 'inner-summarize', title: 'Inner summarize', depends_on: ['inner-fetch'] },
            ],
          },
        },
        { id: 'outer-finish', title: 'Outer finish', depends_on: ['inner-loop'] },
      ],
    },
  };
  const nestedSnapshot = { recipe_name: 'nested', status: 'running', logs: [], edges: [], steps: [nestedLoopStep] };
  const innerLoopNodeID = loopBodyGraphID('outer', 'inner-loop');
  const innerFetchNodeID = loopBodyGraphID(innerLoopNodeID, 'inner-fetch');
  const innerSummarizeNodeID = loopBodyGraphID(innerLoopNodeID, 'inner-summarize');
  const nestedExpanded = computeGraphData(nestedSnapshot, new Set(['outer', innerLoopNodeID]));
  assert.ok(nestedExpanded.nodes.some(node => node.id === innerLoopNodeID), 'nested loop body nodes should render as graph nodes in the parent loop island');
  assert.ok(nestedExpanded.nodes.some(node => node.id === innerFetchNodeID), 'nested loop descendants should render as their own independent graph island');
  assert.ok(nestedExpanded.edges.some(edge => edge.source === innerFetchNodeID && edge.target === innerSummarizeNodeID), 'nested loop body dependencies should render as edges');
  assert.equal(
    nestedExpanded.edges.find(edge => edge.id === `outer-loop-entry-${innerLoopNodeID}`)?.data.kind,
    'loop-expand',
    'outer loop should have a connector edge to its body island entry',
  );
  assert.equal(
    nestedExpanded.edges.find(edge => edge.id === `${innerLoopNodeID}-loop-entry-${innerFetchNodeID}`)?.data.kind,
    'loop-expand',
    'nested loop should have a connector edge to its nested body island entry',
  );
  assert.equal(nestedExpanded.combos.length, 0, 'expanded nested graph should not contain loop combos');
  assert.equal(nestedExpanded.nodes.find(node => node.id === 'outer')?.data.expanded, true, 'outer loop node should still expose expanded state');
  assert.ok(
    nestedExpanded.nodes.find(node => node.id === innerFetchNodeID).style.x
      > nestedExpanded.nodes.find(node => node.id === innerLoopNodeID).style.x,
    'nested loop graph island should be placed to the right of the nested loop node',
  );
  const nestedLive = computeLiveGraphData({
    ...nestedSnapshot,
    execution_instances: [
      { address: 'outer.iter3.inner-loop.iter2.inner-fetch', definition_step_id: 'inner-fetch', parent_loop_id: 'outer', body_step_id: 'inner-fetch', iteration_path: [3, 2], status: 'completed' },
      { address: 'outer.iter3.inner-loop.iter2.inner-summarize', definition_step_id: 'inner-summarize', parent_loop_id: 'outer', body_step_id: 'inner-summarize', iteration_path: [3, 2], status: 'running' },
    ],
  });
  assert.ok(
    nestedLive.edges.some(edge => edge.source === 'outer.iter3.inner-loop.iter2.inner-fetch' && edge.target === 'outer.iter3.inner-loop.iter2.inner-summarize'),
    'live nested iterations should preserve nested body dependencies',
  );

  const selectedFromBody = resolveClickedStep(fetchNodeID, { kind: 'loop-body', parentStep: loopStep, step: fetchBodyStep }, snapshot);
  assert.equal(selectedFromBody?.id, fetchNodeID, 'clicking a loop body graph node should select the body step itself');

  const loopNode = expanded.nodes.find(node => node.id === 'collect');
  const selectedFromLoop = resolveClickedStep('collect', loopNode.data, snapshot);
  assert.equal(selectedFromLoop?.id, 'collect', 'clicking a loop node should select the loop step');

  const crossingSnapshot = {
    recipe_name: 'crossing',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'left-source', title: 'Left source', status: 'pending', index: 0 },
      { id: 'right-source', title: 'Right source', status: 'pending', index: 1 },
      { id: 'right-target', title: 'Right target', status: 'pending', index: 2, depends_on: ['right-source'] },
      { id: 'left-target', title: 'Left target', status: 'pending', index: 3, depends_on: ['left-source'] },
    ],
  };
  const crossingGraph = computeGraphData(crossingSnapshot, new Set());
  assert.ok(crossingGraph.nodes.every(node => Number.isFinite(node.style?.x) && Number.isFinite(node.style?.y)), 'graph model should emit concrete x/y coordinates for every node');
  const rankOneNodes = crossingGraph.nodes
    .filter(node => node.data.layoutRank === 1)
    .sort((a, b) => a.data.layoutOrder - b.data.layoutOrder)
    .map(node => node.id);
  assert.deepEqual(rankOneNodes, ['left-target', 'right-target'], 'same-rank targets should be reordered by predecessor barycenter to avoid avoidable crossings');
  const leftTarget = crossingGraph.nodes.find(node => node.id === 'left-target');
  const rightTarget = crossingGraph.nodes.find(node => node.id === 'right-target');
  assert.ok(leftTarget.style.x < rightTarget.style.x, 'custom layout coordinates should place reordered left target before right target');
  assert.equal(leftTarget.data.layoutX, leftTarget.style.x, 'layoutX should match emitted node style x');
  assert.equal(leftTarget.data.layoutY, leftTarget.style.y, 'layoutY should match emitted node style y');
  const leftEdge = crossingGraph.edges.find(edge => edge.id === 'left-source-left-target');
  const rightEdge = crossingGraph.edges.find(edge => edge.id === 'right-source-right-target');
  assert.equal(leftEdge?.data.sourcePort, 'bottom', 'single left dependency should still use the centered source port');
  assert.equal(rightEdge?.data.targetPort, 'top', 'single right dependency should still use the centered target port');

  const varsSnapshot = {
    recipe_name: 'vars',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'seed', title: 'Seed', status: 'completed', index: 0, output_key: 'topic' },
      { id: 'consume-topic', title: 'Consume topic', status: 'pending', index: 1, input_ctx: ['topic'] },
      { id: 'prepare-repo', title: 'Prepare repo', status: 'pending', index: 2, var_refs: ['repo'] },
      { id: 'scope-analysis', title: 'Scope analysis', status: 'pending', index: 3, input_ctx: ['prepare-repo.stdout.repo_slug'], var_refs: ['repo'] },
      { id: 'repo-map', title: 'Repo map', status: 'pending', index: 4, var_refs: [] },
    ],
  };
  const varsGraph = computeGraphData(varsSnapshot, new Set());
  const topicVar = varsGraph.nodes.find(node => node.id === 'var::topic');
  const repoVar = varsGraph.nodes.find(node => node.id === 'var::repo');
  const prepareRepoVar = varsGraph.nodes.find(node => node.id === 'var::prepare-repo');
  assert.equal(topicVar, undefined, 'input_ctx root matching a step output_key should not create an external variable node');
  assert.equal(repoVar?.data.kind, 'variable', 'template reference should create a variable node');
  assert.deepEqual(repoVar?.data.consumers, ['prepare-repo', 'scope-analysis'], 'template variable node should list every consuming step');
  assert.equal(prepareRepoVar, undefined, 'template reference rooted at a step id should not create an external variable node');
  assert.ok(!varsGraph.edges.some(edge => edge.id === 'var::topic-consume-topic'), 'step output consumption should not create variable consume edge');
  assert.ok(varsGraph.edges.some(edge => edge.id === 'var::repo-prepare-repo' && edge.data.kind === 'variable-consume'), 'template variable should point to prepare-repo');
  assert.ok(varsGraph.edges.some(edge => edge.id === 'var::repo-scope-analysis' && edge.data.kind === 'variable-consume'), 'template variable should point to scope-analysis');
  assert.ok(!varsGraph.edges.some(edge => edge.id === 'var::prepare-repo-repo-map'), 'template refs to step outputs should not create variable consume edges');

  const bodyVarsSnapshot = {
    recipe_name: 'body-vars',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      {
        id: 'loop-step',
        title: 'Loop step',
        status: 'pending',
        index: 0,
        loop: {
          body: [
            { id: 'fetch', title: 'Fetch', var_refs: ['repo'] },
          ],
        },
      },
      { id: 'seed', title: 'Seed', status: 'completed', index: 1 },
    ],
  };
  const bodyVarsGraph = computeGraphData(bodyVarsSnapshot, new Set(['loop-step']));
  const bodyVar = bodyVarsGraph.nodes.find(node => node.id === 'var::repo');
  const scopedBodyVar = bodyVarsGraph.nodes.find(node => node.id === 'loop-step__var::repo');
  assert.equal(bodyVar, undefined, 'loop body template variables should not create global variable nodes in the main layout');
  assert.equal(scopedBodyVar?.data.kind, 'variable', 'loop body template variables should render inside the scoped loop graph island');
  assert.ok(bodyVarsGraph.nodes.some(node => node.id === loopBodyGraphID('loop-step', 'fetch')), 'loop body consumer should render inside the independent loop graph island');
  assert.ok(!bodyVarsGraph.nodes.some(node => node.id === 'var::seed'), 'only backend-provided var_refs should become variable nodes');

  const collisionSnapshot = {
    recipe_name: 'collision',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'prepare-repo', title: 'Prepare repo', status: 'pending', index: 0, var_refs: ['prepare-repo', 'external_var'] },
    ],
  };
  const collisionGraph = computeGraphData(collisionSnapshot, new Set());
  assert.ok(collisionGraph.nodes.some(node => node.id === 'var::prepare-repo'), 'external variable sharing a step id name should still be shown');
  assert.ok(collisionGraph.nodes.some(node => node.id === 'var::external_var'), 'other external variables should still be shown');

  const sharedVarSnapshot = {
    recipe_name: 'shared-var',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'a', title: 'A', status: 'pending', index: 0, var_refs: ['repo'] },
      { id: 'b', title: 'B', status: 'pending', index: 1, var_refs: ['repo'] },
    ],
  };
  const sharedVarGraph = computeGraphData(sharedVarSnapshot, new Set());
  const sharedRepo = sharedVarGraph.nodes.find(node => node.id === 'var::repo');
  assert.deepEqual(sharedRepo?.data.consumers, ['a', 'b'], 'shared variable should fan out to multiple steps');

  const syntaxSnapshot = {
    recipe_name: 'syntax',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'syntax', title: 'Syntax', status: 'pending', index: 0, var_refs: ['repo.ok'] },
    ],
  };
  const syntaxGraph = computeGraphData(syntaxSnapshot, new Set());
  assert.ok(syntaxGraph.nodes.some(node => node.id === 'var::repo'), 'dotted var_ref should be grouped by root');
  assert.ok(!syntaxGraph.nodes.some(node => node.id === 'var::1repo'), 'only backend-provided var_refs should be displayed');

  const rankIsolationSnapshot = {
    recipe_name: 'rank-isolation',
    status: 'pending',
    logs: [],
    edges: [],
    steps: [
      { id: 'independent', title: 'Independent', status: 'pending', index: 0, var_refs: ['repo'] },
      { id: 'downstream', title: 'Downstream', status: 'pending', index: 1, depends_on: ['independent'] },
    ],
  };
  const rankIsolationGraph = computeGraphData(rankIsolationSnapshot, new Set());
  const repoVarNode = rankIsolationGraph.nodes.find(node => node.id === 'var::repo');
  const independentNode = rankIsolationGraph.nodes.find(node => node.id === 'independent');
  const downstreamNode = rankIsolationGraph.nodes.find(node => node.id === 'downstream');
  assert.equal(repoVarNode?.data.layoutRank, 0, 'variable nodes should occupy the first visual rank');
  assert.equal(independentNode?.data.layoutRank, 1, 'steps should start one rank below variable nodes');
  assert.equal(downstreamNode?.data.layoutRank, 2, 'true dependency edge should still determine downstream rank below its parent');

  console.log('formula graph smoke passed');
} finally {
  await rm(outfile, { force: true });
  await rm(executionOutfile, { force: true });
}

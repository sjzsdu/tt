import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';
import { readFile, writeFile } from 'node:fs/promises';

const root = process.cwd();
const entry = join(root, 'src/components/graph/graphModel.ts');
const outfile = join(tmpdir(), `formula-graph-model-smoke-${process.pid}.mjs`);

if (!existsSync(entry)) {
  throw new Error(`Missing graph model entry: ${entry}`);
}

const source = await readFile(entry, 'utf8');
const transpiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
  },
}).outputText;
await writeFile(outfile, transpiled);

try {
  const {
    computeGraphData,
    loopBodyGraphID,
    resolveClickedStep,
    shouldToggleLoopOnClick,
  } = await import(pathToFileURL(outfile));

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
  const collectOutgoing = collapsed.edges.filter(edge => edge.source === 'collect').sort((a, b) => a.target.localeCompare(b.target));
  assert.equal(new Set(collectOutgoing.map(edge => edge.data.sourcePort)).size, collectOutgoing.length, 'same-source edges should use distinct source ports');
  assert.deepEqual(collectOutgoing.map(edge => edge.data.sourcePort).sort(), ['bottom-left-1', 'bottom-right-1'], 'two same-source edges should use balanced ports around the center');
  assert.ok(collectOutgoing.some(edge => edge.data.laneOffset !== 0), 'same-source edges should receive non-zero lane offsets');

  const expanded = computeGraphData(snapshot, new Set(['collect']));
  const fetchNodeID = loopBodyGraphID('collect', 'fetch');
  const summarizeNodeID = loopBodyGraphID('collect', 'summarize');
  const fetchNode = expanded.nodes.find(node => node.id === fetchNodeID);

  assert.equal(expanded.nodes.length, 6, 'expanded graph should include two loop body nodes');
  assert.equal(expanded.combos.length, 1, 'expanded graph should contain one loop combo');
  assert.ok(fetchNode, 'expanded graph should contain fetch loop body');
  assert.equal(fetchNode.data.kind, 'loop-body', 'body node should be marked as loop-body');
  assert.equal(fetchNode.data.step.status, 'completed', 'body node should inherit latest matching activity status');
  assert.equal(fetchNode.data.step.metadata.iteration, '2', 'body node should expose latest iteration metadata');
  const loopSequenceEdge = expanded.edges.find(edge => edge.source === fetchNodeID && edge.target === summarizeNodeID);
  assert.ok(loopSequenceEdge, 'loop body sequence edge should exist');
  assert.equal(loopSequenceEdge.data.kind, 'loop-sequence', 'loop body sequence edge should be visually typed');
  const loopExpandEdge = expanded.edges.find(edge => edge.source === 'collect' && edge.target === fetchNodeID);
  assert.equal(loopExpandEdge?.data.kind, 'loop-expand', 'loop expansion edge should be visually typed');

  const selectedFromBody = resolveClickedStep(fetchNodeID, fetchNode.data, snapshot);
  assert.equal(selectedFromBody?.id, 'collect', 'clicking a synthetic loop body should select its parent loop step');
  assert.equal(shouldToggleLoopOnClick(fetchNodeID, selectedFromBody), false, 'clicking a loop body should not toggle parent expansion');

  const loopNode = expanded.nodes.find(node => node.id === 'collect');
  const selectedFromLoop = resolveClickedStep('collect', loopNode.data, snapshot);
  assert.equal(selectedFromLoop?.id, 'collect', 'clicking a loop node should select the loop step');
  assert.equal(shouldToggleLoopOnClick('collect', selectedFromLoop), true, 'clicking the loop node itself should toggle expansion');

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

  console.log('formula graph smoke passed');
} finally {
  await rm(outfile, { force: true });
}

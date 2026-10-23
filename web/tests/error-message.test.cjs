const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const srcDir = path.resolve(__dirname, '../src')

function sourceFiles(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) return sourceFiles(full)
    return entry.name.endsWith('.tsx') ? [full] : []
  })
}

function loadErrorMessage() {
  const filename = path.join(srcDir, 'components/ErrorMessage.tsx')
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const module = { exports: {} }
  const jsx = (type, props) => ({ type, props })
  new Function('require', 'module', 'exports', source)((name) => {
    if (name === 'react/jsx-runtime') return { jsx, jsxs: jsx }
    throw new Error(`Unexpected import ${name}`)
  }, module, module.exports)
  return module.exports.ErrorMessage
}

test('TestReqUi013_AC01 the shared error component always renders role=alert', () => {
  const element = loadErrorMessage()({ children: 'Etwas ist schiefgelaufen.' })
  assert.equal(element.type, 'p')
  assert.equal(element.props.role, 'alert')
  assert.equal(element.props.className, 'error')
  assert.equal(element.props.children, 'Etwas ist schiefgelaufen.')
})

test('TestReqUi013_AC01 the error style has a red left edge and wraps long text', () => {
  const css = fs.readFileSync(path.join(srcDir, 'index.css'), 'utf8')
  const rules = [...css.matchAll(/(^|\n)\.error \{([^}]*)\}/g)].map((match) => match[2]).join('\n')
  assert.match(rules, /color: var\(--danger\)/)
  assert.match(rules, /border-left: 2px solid var\(--danger\)/)
  assert.match(rules, /overflow-wrap: anywhere/)
})

test('TestReqUi013_AC02 no page or component builds its own error element', () => {
  const offenders = []
  for (const file of sourceFiles(srcDir)) {
    if (path.basename(file) === 'ErrorMessage.tsx') continue
    const text = fs.readFileSync(file, 'utf8')
    const relative = path.relative(srcDir, file)
    if (/className=(["'{`])[^"'`}]*\b(error|addon-hint-error)\b/.test(text)) offenders.push(`${relative}: error class`)
    if (/role=["']alert["']/.test(text) && relative !== path.join('components', 'MFAReminderBanner.tsx')) offenders.push(`${relative}: role="alert"`)
  }
  assert.deepEqual(offenders, [])
})

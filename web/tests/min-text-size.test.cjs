const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')

// DESIGN.md "Minimum size" (DQ-3): no text below 12px.
const files = ['index.css', 'secondary-pages.css']
const sizePattern = /font(?:-size)?\s*:\s*([0-9]*\.?[0-9]+)(px|rem|em)/g

test('CSS declares no font size below 12px', () => {
  const tooSmall = []
  for (const file of files) {
    const css = fs.readFileSync(path.join(__dirname, '..', 'src', file), 'utf8')
    for (const match of css.matchAll(sizePattern)) {
      const px = Number(match[1]) * (match[2] === 'px' ? 1 : 16)
      if (px < 12) tooSmall.push(`${file}: ${match[0]}`)
    }
  }
  assert.deepEqual(tooSmall, [])
})

test('inline font sizes in TSX are at least 12px', () => {
  const tooSmall = []
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) walk(full)
      else if (entry.name.endsWith('.tsx')) {
        for (const match of fs.readFileSync(full, 'utf8').matchAll(/fontSize:\s*([0-9.]+)/g)) {
          if (Number(match[1]) < 12) tooSmall.push(`${entry.name}: ${match[0]}`)
        }
      }
    }
  }
  walk(path.join(__dirname, '..', 'src'))
  assert.deepEqual(tooSmall, [])
})

test('a global small rule keeps unstyled <small> text at 12px or more', () => {
  const css = fs.readFileSync(path.join(__dirname, '..', 'src', 'index.css'), 'utf8')
  const rule = css.match(/(?:^|\n)small\s*\{([^}]*)\}/)
  assert.ok(rule, 'index.css needs a global small rule')
  const size = rule[1].match(/font-size:\s*([0-9.]+)px/)
  assert.ok(size && Number(size[1]) >= 12)
})

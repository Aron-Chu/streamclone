import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const file = path.join(__dirname, '..', 'src', 'components', 'Analytics.tsx')
const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/)
const kept = [...lines.slice(0, 1125), ...lines.slice(2826)]
fs.writeFileSync(file, kept.join('\n'))
console.log('removed lines 1126-2826, new length', kept.length)

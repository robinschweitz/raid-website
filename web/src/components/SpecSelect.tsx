interface Props {
  label: string
  value: string
  specs: string[]
  disabled?: boolean
  required?: boolean
  onChange: (value: string) => void
}

export function SpecSelect({ label, value, specs, disabled, required, onChange }: Props) {
  return <label>{label}<select aria-label={label} disabled={disabled} required={required} value={value} onChange={(event) => onChange(event.target.value)}>
    <option value="">{required ? 'Spec wählen…' : 'Kein Spec'}</option>
    {value && !specs.includes(value) && <option value={value}>{value} (gespeichert)</option>}
    {specs.map((spec) => <option key={spec} value={spec}>{spec}</option>)}
  </select></label>
}

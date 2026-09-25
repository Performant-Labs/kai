// 源语言标签的「自动检测」反馈（issue #11）：源语言为 auto 且引擎已返回检测语言时，
// 语言条不再显示裸的「检测语言」，而是显示「English (detected)」式的反馈——让用户
// 看见系统识别到了什么。用户一旦手动固定了具体语言，反馈即消失（返回 null，调用方
// 回退到 langName(fromLang)）。
//
// nameOf（语言码→展示名）由调用方注入（i18n 的 langName），auto 的语言码同样由调用方
// 传入（TRANSLATE_LANG.Auto）——本模块保持纯函数、不依赖生成的 bindings，vitest 可直接覆盖。

/**
 * 返回源语言下拉框中 auto 选项的显示标签。
 *
 * @param fromLang 当前源语言选择（autoCode 或具体语言码）
 * @param autoCode auto 的语言码（TRANSLATE_LANG.Auto）
 * @param detectedFrom 引擎结果中实际识别出的源语言码（无结果/未检测时为空串）
 * @param nameOf 语言码 → 展示名
 * @param suffix 检测后缀（i18n，如 ' (detected)' / '（已检测）'，可自带前导空格）
 * @returns 覆盖标签；不需要覆盖（已固定语言，或尚无检测结果）时返回 null
 */
export function detectedSourceLabel(
  fromLang: string,
  autoCode: string,
  detectedFrom: string,
  nameOf: (code: string) => string,
  suffix: string,
): string | null {
  if (fromLang !== autoCode) return null;
  if (detectedFrom === '') return null;
  return nameOf(detectedFrom) + suffix;
}

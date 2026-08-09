#!/usr/bin/env bash
set -euo pipefail

destination="${1:-benchmark_results/models}"

download_git_blob() {
    local repository="$1"
    local revision="$2"
    local path="$3"
    local expected_blob="$4"
    local output="${destination}/${repository}/${path}"

    mkdir -p "$(dirname "${output}")"
    curl -L --fail --silent --show-error \
        "https://huggingface.co/${repository}/resolve/${revision}/${path}" \
        -o "${output}.partial"
    local actual_blob
    actual_blob="$(git hash-object "${output}.partial")"
    if [[ "${actual_blob}" != "${expected_blob}" ]]; then
        printf 'hash mismatch for %s/%s\n' "${repository}" "${path}" >&2
        return 1
    fi
    mv "${output}.partial" "${output}"
}

download_lfs_file() {
    local repository="$1"
    local revision="$2"
    local path="$3"
    local expected_sha256="$4"
    local output="${destination}/${repository}/${path}"

    mkdir -p "$(dirname "${output}")"
    curl -L --fail --silent --show-error \
        "https://huggingface.co/${repository}/resolve/${revision}/${path}" \
        -o "${output}.partial"
    local actual_sha256
    actual_sha256="$(sha256sum "${output}.partial" | cut -d ' ' -f 1)"
    if [[ "${actual_sha256}" != "${expected_sha256}" ]]; then
        printf 'hash mismatch for %s/%s\n' "${repository}" "${path}" >&2
        return 1
    fi
    mv "${output}.partial" "${output}"
}

upply_repository="upply-org/bge-small-jobs-data-embedding"
upply_revision="042d48864ea832df6d22abaca1870c6b8d59a07a"
download_git_blob "${upply_repository}" "${upply_revision}" config.json 4382e6b8479cd17913ddcec6c227ebaf63f34a3d
download_git_blob "${upply_repository}" "${upply_revision}" ort_config.json 03d79d95d1b482aa600d4af7649cb91eb4125eec
download_git_blob "${upply_repository}" "${upply_revision}" special_tokens_map.json 9bbecc17cabbcbd3112c14d6982b51403b264bfa
download_git_blob "${upply_repository}" "${upply_revision}" tokenizer.json 3c0e6344ec45a9a6e5a621d6711baf109c2d9f87
download_git_blob "${upply_repository}" "${upply_revision}" tokenizer_config.json 2487f2c85e5018c2e98dfc16bdaa13de236220a1
download_git_blob "${upply_repository}" "${upply_revision}" vocab.txt fb140275c155a9c7c5a3b3e0e77a9e839594a938
download_lfs_file "${upply_repository}" "${upply_revision}" model_quantized.onnx 3f1ca2ff91c049e0b860232d38080f80851942b70ab09b875a2cb64427c844ef

jobbert_repository="TechWolf/JobBERT-v2"
jobbert_revision="a480476925abdf9d97621e56aa38abbb572fe343"
download_git_blob "${jobbert_repository}" "${jobbert_revision}" 1_Pooling/config.json 9213783a33aa495a3c2d3791c7c5888d90607a4e
download_git_blob "${jobbert_repository}" "${jobbert_revision}" 2_Asym/140216480444672_Dense/config.json dd7437dd63d58464a58ff741bc3b573bab5514db
download_git_blob "${jobbert_repository}" "${jobbert_revision}" 2_Asym/140216480445248_Dense/config.json dd7437dd63d58464a58ff741bc3b573bab5514db
download_git_blob "${jobbert_repository}" "${jobbert_revision}" 2_Asym/config.json 8dc3d6c21571f742d7e90c22268553dc48bb87b3
download_git_blob "${jobbert_repository}" "${jobbert_revision}" config.json 51a7c0a4cd9e848d6d017eb7155de9fa67bfa135
download_git_blob "${jobbert_repository}" "${jobbert_revision}" config_sentence_transformers.json b0549abad04419908082afe3364f13c37cbfe1af
download_git_blob "${jobbert_repository}" "${jobbert_revision}" modules.json 354d72158024b8890490124a4345ae55f7f7edd5
download_git_blob "${jobbert_repository}" "${jobbert_revision}" sentence_bert_config.json f789d99277496b282d19020415c5ba9ca79ac875
download_git_blob "${jobbert_repository}" "${jobbert_revision}" special_tokens_map.json f8b386f07f964a5431cc027051fa4487d8b3141c
download_git_blob "${jobbert_repository}" "${jobbert_revision}" tokenizer.json c6c6450c4ccafcecbae9a6a4ce665682b8b58fbe
download_git_blob "${jobbert_repository}" "${jobbert_revision}" tokenizer_config.json 77c49e89c470a889da6b40467e3c893765acb42f
download_git_blob "${jobbert_repository}" "${jobbert_revision}" vocab.txt 1c51ab79a2298a340952d3e6012042a9c84bbe4d
download_lfs_file "${jobbert_repository}" "${jobbert_revision}" model.safetensors 955fda98dcb7765d37617ace3e0a13c8695ac6d4c2e27d4b85c0e9454222117a
download_lfs_file "${jobbert_repository}" "${jobbert_revision}" 2_Asym/140216480444672_Dense/model.safetensors 32b0721292561cc684d5e71dc13b2d5fc9e86405cb085194500fbb0232530e45
download_lfs_file "${jobbert_repository}" "${jobbert_revision}" 2_Asym/140216480445248_Dense/model.safetensors 647eaba5e180e77e55e20b2b1f22cb83a2686bbb0881f6c326627d8d9b5f603d

printf 'Verified pinned model artifacts under %s\n' "${destination}"

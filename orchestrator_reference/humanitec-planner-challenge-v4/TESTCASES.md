# Testcase coverage

Mỗi case chứa Deployment Set hiện tại, Score trước/sau của một workload, Resource Type, Resource Definition, Terraform source thật, Active Resources và expected output/error.

| ID | Trọng tâm | Kết quả |
|---|---|---|
| `01-noop` | Delta rỗng nhưng vẫn dựng/provision graph | accept |
| `02-image-env` | đổi image và environment variable | accept |
| `03-add-sidecar` | thêm sidecar | accept |
| `04-remove-sidecar` | xóa sidecar | accept |
| `05-add-private` | thêm private postgres, params, output placeholder | accept |
| `06-update-private` | sửa resource inputs | accept |
| `07-remove-private` | xóa dependency, Active Resource thành unreferenced | accept |
| `08-add-shared` | thêm shared resource qua Score `id` | accept |
| `09-update-shared` | sửa shared params và `shared` JSON Patch | accept |
| `10-remove-shared` | xóa shared dependency, không destroy | accept |
| `11-preserve-other` | giữ nguyên workload khác | accept |
| `12-shared-consumer` | workload khác dùng shared resource | accept |
| `13-private-to-shared` | đổi identity private → shared | accept |
| `14-matching-specificity` | trọng số criteria | accept |
| `15-definition-reference` | Terraform input đọc output resource khác | accept |
| `16-reference-inherit` | descriptor thiếu class/ID kế thừa current | accept |
| `17-co-provision` | `is_dependent` và `match_dependents` | accept |
| `18-chain-topology` | reference chain nhiều tầng | accept |
| `19-diamond-topology` | nhiều consumer dùng một provider | accept |
| `20-terraform-default` | required/resource input/default | accept |
| `21-active-classification` | existing/new/unreferenced | accept |
| `22-class-selects-revision` | đổi class chọn Definition/revision khác | accept |
| `23-add-workload` | `beforeScore: null` | accept |
| `24-remove-workload` | `afterScore: null` | accept |
| `25-no-match` | không có matching Definition | reject |
| `26-ambiguous-match` | hai Definition đồng hạng | reject |
| `27-unknown-score-output` | output ngoài Resource Type schema | reject |
| `28-missing-tf-input` | thiếu required Terraform variable | reject |
| `29-unknown-rd-output` | Definition đọc output provider không có | reject |
| `30-cycle` | cycle do Resource References | reject |
| `31-combined` | image + sidecar + private + shared + reference + co-provision | accept |
| `32-score-param-reference` | resource param đọc output sibling | accept |
| `33-context-placeholder` | resolve `context.*` vào Terraform input | accept |

Các hidden case nên hoán vị map order, thêm JSON Pointer key có `~`/`/`, dùng nhiều Definition không match, graph rộng hơn, nested params/list, nhiều `.tf` file và `source.path`.


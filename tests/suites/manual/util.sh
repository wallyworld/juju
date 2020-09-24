launch_and_wait_addr_ec2() {
    local name instance_name addr_result

    name=${1}
    instance_name=${2}
    instance_image_id=${3}
    subnet_id=${4}
    sg_id=${5}
    addr_result=${6}

    tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
    instance_id=$(aws ec2 run-instances --image-id "${instance_image_id}" \
        --count 1 \
        --instance-type t2.medium \
        --associate-public-ip-address \
        --tag-specifications "${tags}" \
        --key-name "${name}" \
        --subnet-id "${subnet_id}" \
        --security-group-ids "${sg_id}" \
        --query 'Instances[0].InstanceId' \
        --output text)

    echo "${instance_id}" >> "${TEST_DIR}/ec2-instances"

    aws ec2 wait instance-running --instance-ids "${instance_id}"
    sleep 10

    address=$(aws ec2 describe-instances --instance-ids "${instance_id}" --query 'Reservations[0].Instances[0].PublicDnsName' --output text)

    # shellcheck disable=SC2086
    eval $addr_result="'${address}'"
}

run_cleanup_deploy_manual_aws() {
    set +e

    if [ -f "${TEST_DIR}/ec2-instances" ]; then
        echo "====> Cleaning up EC2 instances"
        while read -r ec2_instance; do
            aws ec2 terminate-instances --instance-ids="${ec2_instance}" >>"${TEST_DIR}/aws_cleanup"
        done < "${TEST_DIR}/ec2-instances"
    fi

    if [ -f "${TEST_DIR}/ec2-key-pairs" ]; then
        echo "====> Cleaning up EC2 key-pairs"
        while read -r ec2_keypair; do
            aws ec2 delete-key-pair --key-name="${ec2_keypair}" >>"${TEST_DIR}/aws_cleanup"
        done < "${TEST_DIR}/ec2-key-pairs"
    fi

    set_verbosity

    echo "====> Completed cleaning up aws"
}
